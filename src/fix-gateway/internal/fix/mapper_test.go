package fix

import (
	"testing"

	"github.com/garudax-platform/decimal"
)

// newOrderMsg builds a minimal FIXMessage for a NewOrderSingle with the given fields.
func newOrderMsg(fields map[int]string) *FIXMessage {
	base := map[int]string{
		TagMsgType:      MsgTypeNewOrderSingle,
		TagClOrdID:      "ORD-001",
		TagSymbol:       "MSE001",
		TagSide:         "1",
		TagOrdType:      "2",
		TagOrderQty:     "100",
		TagPrice:        "150.00",
		TagTimeInForce:  "0",
		TagTransactTime: "20260101-09:30:00.000",
	}
	for k, v := range fields {
		base[k] = v
	}
	return &FIXMessage{Fields: base}
}

// TestMapNewOrderSingle_LimitBuy verifies a limit buy order (Side=1, OrdType=2).
func TestMapNewOrderSingle_LimitBuy(t *testing.T) {
	msg := newOrderMsg(map[int]string{
		TagSide:     "1",
		TagOrdType:  "2",
		TagPrice:    "275.50",
		TagOrderQty: "200",
	})

	order, err := MapNewOrderSingle(msg)
	if err != nil {
		t.Fatalf("MapNewOrderSingle error: %v", err)
	}

	if order.Side != SideBuy {
		t.Errorf("Side: got %q, want %q", order.Side, SideBuy)
	}
	if order.OrderType != OrdTypeLimit {
		t.Errorf("OrdType: got %q, want %q", order.OrderType, OrdTypeLimit)
	}
	if !order.Price.Equal(decimal.MustParse("275.50")) {
		t.Errorf("Price: got %s, want 275.50", order.Price.String())
	}
	if order.Quantity != 200 {
		t.Errorf("Quantity: got %d, want 200", order.Quantity)
	}
	if order.InstrumentID != "MSE001" {
		t.Errorf("InstrumentID: got %q, want MSE001", order.InstrumentID)
	}
	if order.IsShortSell {
		t.Error("IsShortSell: got true, want false for buy")
	}
	if order.TimeInForce != TIFDAY {
		t.Errorf("TimeInForce: got %q, want %q", order.TimeInForce, TIFDAY)
	}
	if order.ClientOrderID != "ORD-001" {
		t.Errorf("ClientOrderID: got %q, want ORD-001", order.ClientOrderID)
	}
}

// TestMapNewOrderSingle_MarketSell verifies a market sell order (Side=2, OrdType=1, no price).
func TestMapNewOrderSingle_MarketSell(t *testing.T) {
	msg := newOrderMsg(map[int]string{
		TagSide:    "2",
		TagOrdType: "1",
		TagPrice:   "0",
	})

	order, err := MapNewOrderSingle(msg)
	if err != nil {
		t.Fatalf("MapNewOrderSingle error: %v", err)
	}

	if order.Side != SideSell {
		t.Errorf("Side: got %q, want %q", order.Side, SideSell)
	}
	if order.OrderType != OrdTypeMarket {
		t.Errorf("OrdType: got %q, want %q", order.OrderType, OrdTypeMarket)
	}
	if !order.Price.IsZero() {
		t.Errorf("Price: got %s, want 0 for market order", order.Price.String())
	}
	if order.IsShortSell {
		t.Error("IsShortSell: got true, want false for regular sell")
	}
}

// TestMapNewOrderSingle_ShortSell verifies a short sell order (Side=5).
func TestMapNewOrderSingle_ShortSell(t *testing.T) {
	msg := newOrderMsg(map[int]string{
		TagSide:    "5",
		TagOrdType: "2",
		TagPrice:   "50.00",
	})

	order, err := MapNewOrderSingle(msg)
	if err != nil {
		t.Fatalf("MapNewOrderSingle error: %v", err)
	}

	if order.Side != SideShortSell {
		t.Errorf("Side: got %q, want %q", order.Side, SideShortSell)
	}
	if !order.IsShortSell {
		t.Error("IsShortSell: got false, want true for short sell")
	}
}

// TestMapNewOrderSingle_ShortSellAlt verifies short sell via Side=6.
func TestMapNewOrderSingle_ShortSellAlt(t *testing.T) {
	msg := newOrderMsg(map[int]string{
		TagSide:    "6",
		TagOrdType: "2",
		TagPrice:   "50.00",
	})

	order, err := MapNewOrderSingle(msg)
	if err != nil {
		t.Fatalf("MapNewOrderSingle error: %v", err)
	}

	if order.Side != SideShortSell {
		t.Errorf("Side: got %q, want %q", order.Side, SideShortSell)
	}
	if !order.IsShortSell {
		t.Error("IsShortSell: got false, want true for side=6 short sell")
	}
}

// TestMapNewOrderSingle_InvalidSide verifies that an unknown side value returns an error.
func TestMapNewOrderSingle_InvalidSide(t *testing.T) {
	msg := newOrderMsg(map[int]string{
		TagSide: "9", // not a valid FIX side
	})

	_, err := MapNewOrderSingle(msg)
	if err == nil {
		t.Fatal("expected error for invalid Side=9, got nil")
	}
}

// TestMapNewOrderSingle_MissingSymbol verifies that a missing Symbol field returns an error.
func TestMapNewOrderSingle_MissingSymbol(t *testing.T) {
	msg := &FIXMessage{
		Fields: map[int]string{
			TagMsgType: MsgTypeNewOrderSingle,
			TagClOrdID: "ORD-001",
			// TagSymbol intentionally omitted
			TagSide:        "1",
			TagOrdType:     "2",
			TagOrderQty:    "100",
			TagTimeInForce: "0",
		},
	}

	_, err := MapNewOrderSingle(msg)
	if err == nil {
		t.Fatal("expected error for missing Symbol, got nil")
	}
}

// TestMapNewOrderSingle_MissingClOrdID verifies that a missing ClOrdID returns an error.
func TestMapNewOrderSingle_MissingClOrdID(t *testing.T) {
	msg := &FIXMessage{
		Fields: map[int]string{
			TagMsgType: MsgTypeNewOrderSingle,
			// TagClOrdID intentionally omitted
			TagSymbol:      "MSE001",
			TagSide:        "1",
			TagOrdType:     "2",
			TagOrderQty:    "100",
			TagTimeInForce: "0",
		},
	}

	_, err := MapNewOrderSingle(msg)
	if err == nil {
		t.Fatal("expected error for missing ClOrdID, got nil")
	}
}

// TestMapNewOrderSingle_InvalidOrdType verifies that an unknown OrdType returns an error.
func TestMapNewOrderSingle_InvalidOrdType(t *testing.T) {
	msg := newOrderMsg(map[int]string{
		TagOrdType: "9", // not a valid FIX OrdType
	})

	_, err := MapNewOrderSingle(msg)
	if err == nil {
		t.Fatal("expected error for invalid OrdType=9, got nil")
	}
}

// TestMapNewOrderSingle_ZeroQty verifies that zero quantity returns an error.
func TestMapNewOrderSingle_ZeroQty(t *testing.T) {
	msg := newOrderMsg(map[int]string{
		TagOrderQty: "0",
	})

	_, err := MapNewOrderSingle(msg)
	if err == nil {
		t.Fatal("expected error for OrderQty=0, got nil")
	}
}

// TestMapNewOrderSingle_NilMessage verifies that a nil message returns an error.
func TestMapNewOrderSingle_NilMessage(t *testing.T) {
	_, err := MapNewOrderSingle(nil)
	if err == nil {
		t.Fatal("expected error for nil message, got nil")
	}
}

// TestMapNewOrderSingle_WrongMsgType verifies rejection of non-D message types.
func TestMapNewOrderSingle_WrongMsgType(t *testing.T) {
	msg := newOrderMsg(map[int]string{
		TagMsgType: MsgTypeLogon, // "A" instead of "D"
	})

	_, err := MapNewOrderSingle(msg)
	if err == nil {
		t.Fatal("expected error for wrong MsgType, got nil")
	}
}

// TestMapNewOrderSingle_TimeInForce verifies all valid TIF mappings.
func TestMapNewOrderSingle_TimeInForce(t *testing.T) {
	cases := []struct {
		fixTIF   string
		expected string
	}{
		{"0", TIFDAY},
		{"1", TIFGTC},
		{"3", TIFIOC},
		{"4", TIFFOK},
		{"6", TIFGTD},
	}

	for _, c := range cases {
		msg := newOrderMsg(map[int]string{TagTimeInForce: c.fixTIF})
		order, err := MapNewOrderSingle(msg)
		if err != nil {
			t.Errorf("TIF %s: unexpected error: %v", c.fixTIF, err)
			continue
		}
		if order.TimeInForce != c.expected {
			t.Errorf("TIF %s: got %q, want %q", c.fixTIF, order.TimeInForce, c.expected)
		}
	}
}

// TestMapExecutionReport verifies that all required FIX tags are present in the built message.
func TestMapExecutionReport(t *testing.T) {
	execMsg := MapExecutionReport(
		"ORD-12345",                 // orderID        → tag 37
		"EXEC-001",                  // execID         → tag 17
		"0",                         // execType       → tag 150
		"0",                         // ordStatus      → tag 39
		SideBuy,                     // side
		500,                         // qty            → tag 38
		decimal.MustParse("275.50"), // price          → tag 6 (AvgPx)
		400,                         // leavesQty      → tag 151
		100,                         // cumQty         → tag 14
	)

	if execMsg == nil {
		t.Fatal("MapExecutionReport returned nil")
	}

	requiredTags := []struct {
		tag  int
		name string
	}{
		{TagMsgType, "MsgType(35)"},
		{TagOrderID, "OrderID(37)"},
		{TagExecID, "ExecID(17)"},
		{TagExecType, "ExecType(150)"},
		{TagOrdStatus, "OrdStatus(39)"},
		{TagOrderQty, "OrderQty(38)"},
		{TagLeavesQty, "LeavesQty(151)"},
		{TagCumQty, "CumQty(14)"},
		{TagAvgPx, "AvgPx(6)"},
		{TagTransactTime, "TransactTime(60)"},
	}

	for _, req := range requiredTags {
		v := GetTag(execMsg, req.tag)
		if v == "" {
			t.Errorf("missing required tag %s in ExecutionReport", req.name)
		}
	}

	// Verify specific field values.
	if got := GetTag(execMsg, TagMsgType); got != MsgTypeExecutionReport {
		t.Errorf("MsgType: got %q, want %q", got, MsgTypeExecutionReport)
	}
	if got := GetTag(execMsg, TagOrderID); got != "ORD-12345" {
		t.Errorf("OrderID: got %q, want ORD-12345", got)
	}
	if got := GetTag(execMsg, TagExecID); got != "EXEC-001" {
		t.Errorf("ExecID: got %q, want EXEC-001", got)
	}
	if got := GetIntTag(execMsg, TagOrderQty); got != 500 {
		t.Errorf("OrderQty: got %d, want 500", got)
	}
	if got := GetIntTag(execMsg, TagLeavesQty); got != 400 {
		t.Errorf("LeavesQty: got %d, want 400", got)
	}
	if got := GetIntTag(execMsg, TagCumQty); got != 100 {
		t.Errorf("CumQty: got %d, want 100", got)
	}

	// Side should be mapped back to FIX "1" for buy.
	if got := GetTag(execMsg, TagSide); got != "1" {
		t.Errorf("Side: got %q, want 1 (BUY)", got)
	}

	// AvgPx is rendered via Decimal.String() — exact, no float drift.
	avgPx := GetTag(execMsg, TagAvgPx)
	expected := decimal.MustParse("275.50").String() // "275.5"
	if avgPx != expected {
		t.Errorf("AvgPx: got %q, want %q", avgPx, expected)
	}
}

// TestMapExecutionReport_SellSide verifies side mapping for sell in an execution report.
func TestMapExecutionReport_SellSide(t *testing.T) {
	execMsg := MapExecutionReport("ORD-999", "EXEC-002", "F", "4", SideSell, 100, decimal.Zero(), 0, 100)
	if got := GetTag(execMsg, TagSide); got != "2" {
		t.Errorf("Side for SELL: got %q, want 2", got)
	}
}

// TestMapExecutionReport_ShortSellSide verifies side mapping for short sell.
func TestMapExecutionReport_ShortSellSide(t *testing.T) {
	execMsg := MapExecutionReport("ORD-999", "EXEC-003", "F", "4", SideShortSell, 50, decimal.Zero(), 0, 50)
	if got := GetTag(execMsg, TagSide); got != "5" {
		t.Errorf("Side for SHORT_SELL: got %q, want 5", got)
	}
}

// TestMapNewOrderSingle_StopOrder verifies Stop order type mapping (OrdType=3).
func TestMapNewOrderSingle_StopOrder(t *testing.T) {
	msg := newOrderMsg(map[int]string{
		TagOrdType: "3",
		TagStopPx:  "100.00",
	})

	order, err := MapNewOrderSingle(msg)
	if err != nil {
		t.Fatalf("MapNewOrderSingle error: %v", err)
	}
	if order.OrderType != OrdTypeStop {
		t.Errorf("OrdType: got %q, want %q", order.OrderType, OrdTypeStop)
	}
	if !order.StopPrice.Equal(decimal.MustParse("100.00")) {
		t.Errorf("StopPrice: got %s, want 100.00", order.StopPrice.String())
	}
}

// TestMapNewOrderSingle_StopLimitOrder verifies StopLimit order type mapping (OrdType=4).
func TestMapNewOrderSingle_StopLimitOrder(t *testing.T) {
	msg := newOrderMsg(map[int]string{
		TagOrdType: "4",
		TagPrice:   "95.00",
		TagStopPx:  "100.00",
	})

	order, err := MapNewOrderSingle(msg)
	if err != nil {
		t.Fatalf("MapNewOrderSingle error: %v", err)
	}
	if order.OrderType != OrdTypeStopLimit {
		t.Errorf("OrdType: got %q, want %q", order.OrderType, OrdTypeStopLimit)
	}
}

// TestMapNewOrderSingle_WithAccount verifies that Account field is captured.
func TestMapNewOrderSingle_WithAccount(t *testing.T) {
	msg := newOrderMsg(map[int]string{
		TagAccount: "ACC-XYZ",
	})

	order, err := MapNewOrderSingle(msg)
	if err != nil {
		t.Fatalf("MapNewOrderSingle error: %v", err)
	}
	if order.Account != "ACC-XYZ" {
		t.Errorf("Account: got %q, want ACC-XYZ", order.Account)
	}
}
