// Package kafkae2e is a cross-process integration test (R024) that verifies real
// Kafka event propagation across the four trading engines:
//
//	matching-engine  -> ace-commodities.trades.executed
//	clearing-engine  -> ace-commodities.clearing.novated
//	margin-engine    -> ace-commodities.margin.call-issued
//	settlement-engine-> ace-commodities.settlement.completed
//
// It publishes a synthetic trade-executed event to the matching topic and
// asserts that the downstream engines (running as separate processes against the
// same broker, with KAFKA_BROKERS set) consume it and emit their own events,
// correlated by the original correlation ID.
//
// This test requires a running full stack (docker compose) with a real broker.
// It GRACEFULLY SKIPS when KAFKA_BROKERS is unset or the broker is unreachable,
// so it is safe to keep in CI alongside unit tests (the documented graceful-skip
// pattern). A FAILURE here in full-stack mode means the cross-service wiring
// regressed — it is not a test bug.
package kafkae2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	kafkago "github.com/segmentio/kafka-go"
)

const tenant = "ace-commodities"

const (
	topicTradesExecuted     = tenant + ".trades.executed"
	topicClearingNovated    = tenant + ".clearing.novated"
	topicMarginCallIssued   = tenant + ".margin.call-issued"
	topicSettlementComplete = tenant + ".settlement.completed"
)

// event mirrors the GarudaX kafka.Event envelope on the wire.
type event struct {
	ID            string          `json:"id"`
	Type          string          `json:"type"`
	Timestamp     time.Time       `json:"timestamp"`
	Source        string          `json:"source"`
	CorrelationID string          `json:"correlation_id"`
	SchemaVersion int             `json:"schema_version"`
	Payload       json.RawMessage `json:"payload"`
}

type tradeExecutedPayload struct {
	TradeID             string `json:"trade_id"`
	InstrumentID        string `json:"instrument_id"`
	BuyOrderID          string `json:"buy_order_id"`
	SellOrderID         string `json:"sell_order_id"`
	BuyerParticipantID  string `json:"buyer_participant_id"`
	SellerParticipantID string `json:"seller_participant_id"`
	Price               string `json:"price"`
	Quantity            uint64 `json:"quantity"`
	TradeValue          string `json:"trade_value"`
	AggressorSide       string `json:"aggressor_side"`
	TradeType           string `json:"trade_type"`
	SequenceNumber      uint64 `json:"sequence_number"`
	ExecutedAt          string `json:"executed_at"`
}

func brokers() []string {
	v := strings.TrimSpace(os.Getenv("KAFKA_BROKERS"))
	if v == "" {
		return nil
	}
	return strings.Split(v, ",")
}

// skipIfBrokerUnavailable skips the test unless a broker is configured AND
// reachable. This keeps the test inert in unit-only CI runs.
func skipIfBrokerUnavailable(t *testing.T) []string {
	t.Helper()
	bs := brokers()
	if len(bs) == 0 {
		t.Skip("KAFKA_BROKERS not set; skipping cross-service Kafka propagation test")
	}
	conn, err := net.DialTimeout("tcp", bs[0], 3*time.Second)
	if err != nil {
		t.Skipf("Kafka broker %s unreachable (%v); skipping", bs[0], err)
	}
	_ = conn.Close()
	return bs
}

// watcher consumes a topic from a fresh consumer group (start at last offset, so
// only messages produced after it joins are seen) and reports the first message
// whose correlation ID matches want.
//
// Matching is by CONTAINMENT, not strict equality (R027): the original
// correlation ID (a unique nanosecond-based token) is preserved verbatim by the
// clearing engine on clearing.novated, but the settlement engine re-keys its
// downstream event with the deterministic idempotency key "cycle-<tradeID>"
// (the R024 deterministic-key pattern) and carries that as the correlation ID
// on settlement.completed. A strict "==" match therefore misses the genuine,
// demonstrably-propagated settlement event (verified on the live broker:
// settlement.completed carries correlation_id "cycle-"+want). Containment on the
// unique token keeps the assertion specific while tolerating the documented
// "cycle-" prefix, so the test reflects real propagation rather than the exact
// correlation-ID encoding.
func watcher(ctx context.Context, t *testing.T, bs []string, topic, group, want string, found chan<- string) {
	r := kafkago.NewReader(kafkago.ReaderConfig{
		Brokers:     bs,
		GroupID:     group,
		Topic:       topic,
		StartOffset: kafkago.LastOffset,
		MinBytes:    1,
		MaxBytes:    10e6,
	})
	defer r.Close()
	for {
		msg, err := r.FetchMessage(ctx)
		if err != nil {
			return // ctx cancelled or reader closed
		}
		_ = r.CommitMessages(ctx, msg)
		var e event
		if err := json.Unmarshal(msg.Value, &e); err != nil {
			continue
		}
		if strings.Contains(e.CorrelationID, want) {
			select {
			case found <- topic:
			default:
			}
			return
		}
	}
}

func TestCrossServiceTradePropagation(t *testing.T) {
	bs := skipIfBrokerUnavailable(t)

	corr := fmt.Sprintf("r024-e2e-%d", time.Now().UnixNano())
	instrument := os.Getenv("E2E_INSTRUMENT")
	if instrument == "" {
		instrument = "WHT-HRW-2026M07-UB"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	// Start the downstream watchers BEFORE producing so their consumer groups
	// join and pin their start offset at LastOffset, guaranteeing they observe
	// the engines' subsequently-emitted events.
	//
	// clearing.novated and settlement.completed are REQUIRED: a novation always
	// produces obligations, and a settlement cycle always completes — so these
	// firing proves matching->clearing and clearing->settlement propagation.
	// margin.call-issued is BEST-EFFORT: the margin engine only emits it when the
	// novated position is actually under-collateralised given the running
	// engine's params, which is deployment-dependent. We log it if seen but do
	// not fail the test on its absence (that would conflate "no call issued" with
	// "event did not propagate").
	required := []struct {
		topic string
		label string
	}{
		{topicClearingNovated, "clearing.novated"},
		{topicSettlementComplete, "settlement.completed"},
	}
	found := make(chan string, len(required)+1)
	for _, d := range required {
		go watcher(ctx, t, bs, d.topic, corr+"-"+d.label, corr, found)
	}
	// Best-effort margin watcher (does not gate the verdict).
	go watcher(ctx, t, bs, topicMarginCallIssued, corr+"-margin.call-issued", corr, found)

	// Give the watcher groups time to join and resolve offsets before producing.
	time.Sleep(5 * time.Second)

	payload := tradeExecutedPayload{
		TradeID:             corr,
		InstrumentID:        instrument,
		BuyOrderID:          "B-" + corr,
		SellOrderID:         "S-" + corr,
		BuyerParticipantID:  "P-BUY",
		SellerParticipantID: "P-SELL",
		Price:               "250.50",
		Quantity:            10,
		TradeValue:          "2505.00",
		AggressorSide:       "BUY",
		TradeType:           "continuous",
		SequenceNumber:      1,
		ExecutedAt:          time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := produceTrade(ctx, bs, corr, payload); err != nil {
		t.Fatalf("produce trade-executed: %v", err)
	}
	t.Logf("published trades.executed corr=%s instrument=%s", corr, instrument)

	got := map[string]bool{}
	requiredCount := func() int {
		n := 0
		for _, d := range required {
			if got[d.topic] {
				n++
			}
		}
		return n
	}
	for requiredCount() < len(required) {
		select {
		case topic := <-found:
			got[topic] = true
			if topic == topicMarginCallIssued {
				t.Logf("observed (best-effort) margin.call-issued propagation")
			} else {
				t.Logf("observed REQUIRED propagation on %s (%d/%d)", topic, requiredCount(), len(required))
			}
		case <-ctx.Done():
			var missing []string
			for _, d := range required {
				if !got[d.topic] {
					missing = append(missing, d.topic)
				}
			}
			t.Fatalf("cross-service propagation incomplete; missing required %v "+
				"(is the instrument %q registered in the running matching-engine, "+
				"and are all engines deployed with KAFKA_BROKERS set?)", missing, instrument)
		}
	}
	if !got[topicMarginCallIssued] {
		t.Logf("note: no margin.call-issued observed (position not under-collateralised in the running margin-engine config) — not a propagation failure")
	}
}

func produceTrade(ctx context.Context, bs []string, key string, payload tradeExecutedPayload) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	e := event{
		ID:            key,
		Type:          topicTradesExecuted,
		Timestamp:     time.Now().UTC(),
		Source:        "r024-kafka-e2e",
		CorrelationID: key,
		SchemaVersion: 1,
		Payload:       raw,
	}
	data, err := json.Marshal(e)
	if err != nil {
		return err
	}
	w := &kafkago.Writer{
		Addr:         kafkago.TCP(bs...),
		Topic:        topicTradesExecuted,
		Balancer:     &kafkago.Hash{},
		RequiredAcks: kafkago.RequireOne,
	}
	defer w.Close()
	wctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return w.WriteMessages(wctx, kafkago.Message{
		Key:   []byte(payload.InstrumentID),
		Value: data,
	})
}
