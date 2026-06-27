// R029 — Live, seeded, gateway-driven economic-completion proof.
//
// Unlike TestCrossServiceTradePropagation (which hand-injects a synthetic
// trades.executed event with hard-coded P-BUY/P-SELL participant IDs to prove
// transport), this test drives a REAL order pair THROUGH THE GATEWAY
// (authenticated, tenant-scoped) that crosses and matches inside the running
// matching-engine. It then proves the resulting trade flows end-to-end across
// the live broker:
//
//	matching  -> ace-commodities.trades.executed   (with NON-EMPTY buyer/seller participant IDs)
//	clearing  -> ace-commodities.clearing.novated   (no "participant IDs are required" error)
//	margin    -> ace-commodities.margin.call-issued  (for the seeded instrument)
//	settlement-> ace-commodities.settlement.completed
//
// This is the assertion R027 asked for: it proves the R028 D1 data flow
// (matching publishes populated participant/order IDs, not the synthetic
// payload's hand-set IDs) and that the seeded margin risk params (R028 D2) and
// the DLQ/cross-service topics (R028 D3) make the full economic lifecycle
// complete for a genuine trade.
//
// It GRACEFULLY SKIPS unless BOTH a Kafka broker (KAFKA_BROKERS) AND the gateway
// (GATEWAY_URL / E2E_BASE_URL) are reachable, so it is safe to keep in CI
// alongside the unit suites. A FAILURE in full-stack mode reports exactly which
// hop broke — it is not a test bug.
package kafkae2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	kafkago "github.com/segmentio/kafka-go"
)

// ---------- gateway HTTP helpers ----------

func gatewayBaseURL() string {
	for _, k := range []string{"GATEWAY_URL", "E2E_BASE_URL"} {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return strings.TrimRight(v, "/")
		}
	}
	return "http://localhost:8080"
}

const seededTenant = "ace-commodities"

// gwDo issues an authenticated, tenant-scoped JSON request to the gateway.
func gwDo(t *testing.T, base, method, path, token string, body interface{}) (int, map[string]interface{}, []byte) {
	t.Helper()
	var r io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal %s %s: %v", method, path, err)
		}
		r = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, base+path, r)
	if err != nil {
		t.Fatalf("new request %s %s: %v", method, path, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GarudaX-Tenant", seededTenant)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("do %s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	var obj map[string]interface{}
	if len(data) > 0 {
		_ = json.Unmarshal(data, &obj)
	}
	return resp.StatusCode, obj, data
}

// gatewayReachable returns true when the gateway health endpoint answers 200.
func gatewayReachable(base string) bool {
	resp, err := (&http.Client{Timeout: 3 * time.Second}).Get(base + "/healthz")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// registerAndLogin creates a trader through the gateway and returns its access
// token. Skips the test if the auth path is unavailable.
func registerAndLogin(t *testing.T, base, role string) string {
	t.Helper()
	email := fmt.Sprintf("r029-%s-%d@e2e-test.ace", role, time.Now().UnixNano())
	const pw = "SeededPass123!"

	code, _, raw := gwDo(t, base, "POST", "/api/v1/auth/register", "", map[string]string{
		"email": email, "password": pw, "role": role,
	})
	if code == http.StatusServiceUnavailable || code == http.StatusBadGateway {
		t.Skipf("auth-service unavailable (register -> %d)", code)
	}
	if code != http.StatusCreated && code != http.StatusOK && code != http.StatusConflict {
		t.Fatalf("register %s failed: %d %s", email, code, raw)
	}

	code, body, raw := gwDo(t, base, "POST", "/api/v1/auth/login", "", map[string]string{
		"email": email, "password": pw,
	})
	if code == http.StatusServiceUnavailable || code == http.StatusBadGateway {
		t.Skipf("auth-service unavailable (login -> %d)", code)
	}
	if code != http.StatusOK {
		t.Fatalf("login %s failed: %d %s", email, code, raw)
	}
	token, _ := body["access_token"].(string)
	if token == "" {
		token, _ = body["AccessToken"].(string)
	}
	if token == "" {
		t.Fatalf("no access token in login response: %s", raw)
	}
	return token
}

// ---------- live-broker collector ----------

// collector consumes a topic from a fresh consumer group and accumulates every
// raw message it sees. It reads from FirstOffset (the beginning of the topic),
// not LastOffset: a group reading from "latest" races the producer — if the
// rebalance for a brand-new group completes AFTER the event is produced, the
// reader seeks past it and silently skips it. Reading from the start is race-free
// on a freshly brought-up stack (the only traffic is this test's), and matching
// is by a UNIQUE per-run token (the run's participant ID / the server-assigned
// trade_id), so historical events from a prior run never false-match.
type collector struct {
	mu   sync.Mutex
	seen [][]byte
}

func newCollector(ctx context.Context, t *testing.T, bs []string, topic, group string) *collector {
	c := &collector{}
	r := kafkago.NewReader(kafkago.ReaderConfig{
		Brokers:     bs,
		GroupID:     group,
		Topic:       topic,
		StartOffset: kafkago.FirstOffset,
		MinBytes:    1,
		MaxBytes:    10e6,
	})
	go func() {
		defer r.Close()
		for {
			msg, err := r.FetchMessage(ctx)
			if err != nil {
				return // ctx cancelled / reader closed
			}
			_ = r.CommitMessages(ctx, msg)
			c.mu.Lock()
			c.seen = append(c.seen, msg.Value)
			c.mu.Unlock()
		}
	}()
	return c
}

// firstMatch returns the first accumulated message whose decoded envelope or raw
// bytes contain the wanted token, or nil if none yet.
func (c *collector) firstMatch(want string) []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, m := range c.seen {
		var e event
		if json.Unmarshal(m, &e) == nil && strings.Contains(e.CorrelationID, want) {
			return m
		}
		if bytes.Contains(m, []byte(want)) {
			return m
		}
	}
	return nil
}

// waitForMatch polls a collector until a message containing want appears or the
// deadline passes.
func waitForMatch(ctx context.Context, c *collector, want string) []byte {
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	for {
		if m := c.firstMatch(want); m != nil {
			return m
		}
		select {
		case <-ctx.Done():
			return c.firstMatch(want)
		case <-tick.C:
		}
	}
}

func TestSeededGatewayTradeSettles(t *testing.T) {
	bs := skipIfBrokerUnavailable(t)

	base := gatewayBaseURL()
	if !gatewayReachable(base) {
		t.Skipf("gateway not reachable at %s; skipping seeded gateway trade test", base)
	}

	instrument := os.Getenv("E2E_INSTRUMENT")
	if instrument == "" {
		instrument = "WHT-HRW-2026M07-UB"
	}

	stamp := time.Now().UnixNano()
	buyerPID := fmt.Sprintf("P-R029-BUY-%d", stamp)
	sellerPID := fmt.Sprintf("P-R029-SELL-%d", stamp)

	// Authenticate two traders through the gateway (proves the request is
	// authenticated + tenant-scoped, not a raw engine call).
	buyerToken := registerAndLogin(t, base, "trader")
	sellerToken := registerAndLogin(t, base, "trader")

	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
	defer cancel()

	// Start the live collectors BEFORE submitting any order.
	tradesCol := newCollector(ctx, t, bs, topicTradesExecuted, fmt.Sprintf("r029-trades-%d", stamp))
	novatedCol := newCollector(ctx, t, bs, topicClearingNovated, fmt.Sprintf("r029-novated-%d", stamp))
	marginCol := newCollector(ctx, t, bs, topicMarginCallIssued, fmt.Sprintf("r029-margin-%d", stamp))
	settleCol := newCollector(ctx, t, bs, topicSettlementComplete, fmt.Sprintf("r029-settle-%d", stamp))

	// Give the consumer groups time to join and pin their offsets.
	time.Sleep(6 * time.Second)

	// Submit the resting buy, then the crossing sell. Both carry distinct,
	// known participant IDs in the body — the gateway forwards them to matching,
	// which (R028 D1) must populate Buyer/SellerParticipantID on the matched
	// trade. We do NOT hand-set anything on the Kafka payload: the IDs reach the
	// broker only by flowing gateway -> matching -> eventbus.
	order := func(token, side, pid string) {
		code, _, raw := gwDo(t, base, "POST", "/api/v1/orders", token, map[string]interface{}{
			"instrument_id":  instrument,
			"side":           side,
			"order_type":     "LIMIT",
			"quantity":       "10",
			"price":          "250.50",
			"participant_id": pid,
			"time_in_force":  "GTC",
		})
		if code == http.StatusServiceUnavailable || code == http.StatusBadGateway {
			t.Skipf("matching-engine unavailable (order %s -> %d)", side, code)
		}
		if code < 200 || code >= 300 {
			t.Fatalf("submit %s order failed: %d %s", side, code, raw)
		}
		t.Logf("submitted %s order pid=%s instrument=%s (HTTP %d)", side, pid, instrument, code)
	}
	order(buyerToken, "BUY", buyerPID)
	time.Sleep(1 * time.Second) // let the buy rest on the book before the sell crosses
	order(sellerToken, "SELL", sellerPID)

	// ----- Hop 1: matching -> trades.executed with NON-EMPTY participant IDs -----
	// Match by the unique per-run buyer participant ID so we observe THIS run's
	// trade (not a residual one from a prior run on the same topic).
	tradeMsg := waitForMatch(ctx, tradesCol, buyerPID)
	if tradeMsg == nil {
		t.Fatalf("HOP 1 BROKE: no trades.executed observed for instrument %q on the live broker. "+
			"The gateway-submitted order pair did not match in the running matching-engine "+
			"(is %q in INSTRUMENTS and continuously trading?)", instrument, instrument)
	}
	var tradeEvt event
	if err := json.Unmarshal(tradeMsg, &tradeEvt); err != nil {
		t.Fatalf("HOP 1: decode trades.executed envelope: %v\nraw: %s", err, tradeMsg)
	}
	var tp tradeExecutedPayload
	if err := json.Unmarshal(tradeEvt.Payload, &tp); err != nil {
		t.Fatalf("HOP 1: decode trades.executed payload: %v\nraw: %s", err, tradeEvt.Payload)
	}
	t.Logf("HOP 1 OK: trades.executed trade_id=%s corr=%s buyer=%q seller=%q buy_order=%q sell_order=%q",
		tp.TradeID, tradeEvt.CorrelationID, tp.BuyerParticipantID, tp.SellerParticipantID, tp.BuyOrderID, tp.SellOrderID)

	// D1 assertion: the published trade MUST carry non-empty participant + order
	// IDs (the pre-R028 bug published empty IDs, which clearing novation rejected).
	if tp.BuyerParticipantID == "" || tp.SellerParticipantID == "" {
		t.Fatalf("HOP 1 BROKE (R028 D1 regression): trades.executed has EMPTY participant IDs "+
			"(buyer=%q seller=%q) — clearing novation will reject this trade", tp.BuyerParticipantID, tp.SellerParticipantID)
	}
	if tp.BuyOrderID == "" || tp.SellOrderID == "" {
		t.Fatalf("HOP 1 BROKE (R028 D1 regression): trades.executed has EMPTY order IDs "+
			"(buy_order=%q sell_order=%q)", tp.BuyOrderID, tp.SellOrderID)
	}
	if tp.BuyerParticipantID != buyerPID || tp.SellerParticipantID != sellerPID {
		t.Errorf("HOP 1: participant IDs did not flow through as submitted: "+
			"got buyer=%q seller=%q, want buyer=%q seller=%q",
			tp.BuyerParticipantID, tp.SellerParticipantID, buyerPID, sellerPID)
	}

	tradeID := tp.TradeID
	if tradeID == "" {
		t.Fatal("HOP 1: trades.executed payload has empty trade_id; cannot correlate downstream hops")
	}

	// ----- Hop 2: clearing -> novated (REQUIRED) -----
	// Matched on the unique per-run participant ID, which clearing.novated carries
	// in its buyer/seller participant fields (the same fields the margin engine
	// consumes); this also confirms the populated IDs survived novation.
	novMsg := waitForMatch(ctx, novatedCol, buyerPID)
	if novMsg == nil {
		t.Fatalf("HOP 2 BROKE: no clearing.novated observed for participant=%s (trade_id=%s). "+
			"matching->clearing propagation failed, or novation rejected the trade "+
			"(participant-IDs-required error) despite populated IDs.", buyerPID, tradeID)
	}
	if bytes.Contains(novMsg, []byte("participant IDs are required")) ||
		bytes.Contains(novMsg, []byte("participant id")) {
		t.Fatalf("HOP 2 BROKE: clearing.novated carries a participant-ID rejection: %s", novMsg)
	}
	t.Logf("HOP 2 OK: clearing.novated observed for participant=%s (trade_id=%s)", buyerPID, tradeID)

	// ----- Hop 4: settlement -> completed (REQUIRED), correlated by trade_id -----
	// (settlement.completed re-keys correlation to "cycle-<tradeID>"; trade_id
	// containment matches it either way.)
	setMsg := waitForMatch(ctx, settleCol, tradeID)
	if setMsg == nil {
		t.Fatalf("HOP 4 BROKE: no settlement.completed observed for trade_id=%s. "+
			"clearing->settlement propagation failed.", tradeID)
	}
	t.Logf("HOP 4 OK: settlement.completed observed for trade_id=%s", tradeID)

	// ----- Hop 3: margin -> call-issued (REQUIRED per R029 verdict) -----
	// R028 D2 seeds risk params for the instrument so a margin call IS issued for
	// the novated position. The R029 verdict requires BOTH margin.call-issued AND
	// settlement.completed, so unlike the transport-only test this margin hop
	// gates the verdict.
	//
	// margin.call-issued is correlated by the margin-engine's own CallID, not the
	// trade_id (the margin event carries no trade_id), so we match on the UNIQUE
	// per-run participant ID — which the margin payload's participant_id field
	// carries verbatim from the novated position's leg.
	marMsg := waitForMatch(ctx, marginCol, buyerPID)
	if marMsg == nil {
		t.Fatalf("HOP 3 BROKE: no margin.call-issued observed for participant=%s (trade_id=%s). "+
			"The seeded margin risk params (R028 D2) for %q may be missing in the running "+
			"margin-engine, or clearing->margin propagation failed.", buyerPID, tradeID, instrument)
	}
	t.Logf("HOP 3 OK: margin.call-issued observed for participant=%s (trade_id=%s)", buyerPID, tradeID)

	t.Logf("VERDICT: CONFIRMED — a real gateway-submitted trade (trade_id=%s, buyer=%s, seller=%s) "+
		"settled end-to-end across the live broker: trades.executed -> clearing.novated -> "+
		"margin.call-issued -> settlement.completed, with populated participant/order IDs.",
		tradeID, buyerPID, sellerPID)
}
