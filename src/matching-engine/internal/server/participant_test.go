package server

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/garudax-platform/matching-engine/internal/types"
)

// crossOnce submits a resting sell and an aggressing buy that fully cross,
// then returns the single resulting trade from the store. (R028 D1)
func crossViaServer(t *testing.T, s *Server, sell, buy SubmitOrderRequest) types.Trade {
	t.Helper()
	if _, err := s.SubmitOrder(sell); err != nil {
		t.Fatalf("submit sell: %v", err)
	}
	if _, err := s.SubmitOrder(buy); err != nil {
		t.Fatalf("submit buy: %v", err)
	}
	trades := s.tradeStore.Trades(sell.InstrumentID)
	if len(trades) != 1 {
		t.Fatalf("expected exactly 1 trade, got %d", len(trades))
	}
	return trades[0]
}

// TestSubmitOrderPopulatesParticipantIDOnTrade is the core R028 D1 assertion:
// a matched trade must carry both participant IDs (clearing novation rejects
// empty IDs) and non-empty order IDs.
func TestSubmitOrderPopulatesParticipantIDOnTrade(t *testing.T) {
	s := newTestServer()
	s.RegisterInstrument("WHT-HRW-2026M07-UB")

	trade := crossViaServer(t, s,
		SubmitOrderRequest{
			OrderID: "sell-1", InstrumentID: "WHT-HRW-2026M07-UB",
			AccountID: "P-SELL", Side: types.SideSell,
			OrderType: types.OrderTypeLimit, TimeInForce: types.TIFDay,
			Price: "100", Quantity: 10,
		},
		SubmitOrderRequest{
			OrderID: "buy-1", InstrumentID: "WHT-HRW-2026M07-UB",
			AccountID: "P-BUY", Side: types.SideBuy,
			OrderType: types.OrderTypeLimit, TimeInForce: types.TIFDay,
			Price: "100", Quantity: 10,
		},
	)

	if trade.BuyerParticipantID != "P-BUY" {
		t.Errorf("BuyerParticipantID = %q, want P-BUY", trade.BuyerParticipantID)
	}
	if trade.SellerParticipantID != "P-SELL" {
		t.Errorf("SellerParticipantID = %q, want P-SELL", trade.SellerParticipantID)
	}
	if trade.BuyOrderID == "" || trade.SellOrderID == "" {
		t.Errorf("order IDs must be non-empty: buy=%q sell=%q", trade.BuyOrderID, trade.SellOrderID)
	}
}

// TestSubmitOrderExplicitParticipantID verifies an explicit ParticipantID is
// preferred over AccountID when both are supplied.
func TestSubmitOrderExplicitParticipantID(t *testing.T) {
	s := newTestServer()
	s.RegisterInstrument("WHEAT")

	trade := crossViaServer(t, s,
		SubmitOrderRequest{
			OrderID: "s1", InstrumentID: "WHEAT",
			AccountID: "acct-sell", ParticipantID: "PART-SELL", Side: types.SideSell,
			OrderType: types.OrderTypeLimit, TimeInForce: types.TIFDay,
			Price: "100", Quantity: 5,
		},
		SubmitOrderRequest{
			OrderID: "b1", InstrumentID: "WHEAT",
			AccountID: "acct-buy", ParticipantID: "PART-BUY", Side: types.SideBuy,
			OrderType: types.OrderTypeLimit, TimeInForce: types.TIFDay,
			Price: "100", Quantity: 5,
		},
	)

	if trade.BuyerParticipantID != "PART-BUY" {
		t.Errorf("BuyerParticipantID = %q, want PART-BUY", trade.BuyerParticipantID)
	}
	if trade.SellerParticipantID != "PART-SELL" {
		t.Errorf("SellerParticipantID = %q, want PART-SELL", trade.SellerParticipantID)
	}
}

// TestSubmitOrderGeneratesOrderIDWhenEmpty verifies the engine assigns an
// OrderID when the caller omits it, so trade order IDs are never blank.
func TestSubmitOrderGeneratesOrderIDWhenEmpty(t *testing.T) {
	s := newTestServer()
	s.RegisterInstrument("WHEAT")

	trade := crossViaServer(t, s,
		SubmitOrderRequest{
			InstrumentID: "WHEAT", AccountID: "P-SELL", Side: types.SideSell,
			OrderType: types.OrderTypeLimit, TimeInForce: types.TIFDay,
			Price: "100", Quantity: 7,
		},
		SubmitOrderRequest{
			InstrumentID: "WHEAT", AccountID: "P-BUY", Side: types.SideBuy,
			OrderType: types.OrderTypeLimit, TimeInForce: types.TIFDay,
			Price: "100", Quantity: 7,
		},
	)

	if trade.BuyOrderID == "" || trade.SellOrderID == "" {
		t.Errorf("engine must assign order IDs when omitted: buy=%q sell=%q",
			trade.BuyOrderID, trade.SellOrderID)
	}
}

// TestHandleSubmitOrderUsesParticipantHeader verifies the HTTP path resolves
// the participant from the X-Participant-Id header (the identity the gateway
// forwards from the JWT claim) and carries it onto the matched trade.
func TestHandleSubmitOrderUsesParticipantHeader(t *testing.T) {
	s := newTestServer()
	s.RegisterInstrument("WHEAT")

	submit := func(side, participant string) {
		body := `{"instrument_id":"WHEAT","side":"` + side +
			`","type":"limit","quantity":"4","price":"100","time_in_force":"day"}`
		req := httptest.NewRequest("POST", "/orders", strings.NewReader(body))
		req.Header.Set("X-Participant-Id", participant)
		w := httptest.NewRecorder()
		s.handleSubmitOrder(w, req)
		if w.Code != 200 {
			t.Fatalf("submit %s: status %d body %s", side, w.Code, w.Body.String())
		}
	}

	submit("sell", "P-SELL")
	submit("buy", "P-BUY")

	trades := s.tradeStore.Trades("WHEAT")
	if len(trades) != 1 {
		t.Fatalf("expected 1 trade, got %d", len(trades))
	}
	tr := trades[0]
	if tr.BuyerParticipantID != "P-BUY" || tr.SellerParticipantID != "P-SELL" {
		t.Errorf("participant IDs from header not propagated: buyer=%q seller=%q",
			tr.BuyerParticipantID, tr.SellerParticipantID)
	}
}
