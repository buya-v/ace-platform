package novation

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/garudax-platform/clearing-engine/internal/types"
)

// seqIDGen provides deterministic sequential IDs for testing.
type seqIDGen struct{ counter uint64 }

func (g *seqIDGen) NewID() string {
	n := atomic.AddUint64(&g.counter, 1)
	return fmt.Sprintf("obl-%d", n)
}

func makeTradeWithInstrument(id, buyer, seller, instrument string, price int64, qty uint64) types.Trade {
	return types.Trade{
		TradeID:             id,
		InstrumentID:        instrument,
		BuyOrderID:          "buy-" + id,
		SellOrderID:         "sell-" + id,
		BuyerParticipantID:  buyer,
		SellerParticipantID: seller,
		Price:               types.DecimalFromInt(price),
		Quantity:            qty,
		TradeValue:          types.DecimalFromInt(price).MulUint64(qty),
		AggressorSide:       types.SideBuy,
		SequenceNumber:      1,
		ExecutedAt:          time.Now(),
	}
}

func TestNovateTableDrivenValidation(t *testing.T) {
	tests := []struct {
		name    string
		trade   types.Trade
		wantErr bool
		errMsg  string
	}{
		{
			name:    "empty trade ID",
			trade:   makeTrade("", "buyer", "seller", 100, 5),
			wantErr: true,
			errMsg:  "trade ID is required",
		},
		{
			name:    "zero quantity",
			trade:   makeTrade("t-1", "buyer", "seller", 100, 0),
			wantErr: true,
			errMsg:  "trade quantity must be positive",
		},
		{
			name:    "empty buyer",
			trade:   makeTrade("t-1", "", "seller", 100, 5),
			wantErr: true,
			errMsg:  "both buyer and seller participant IDs are required",
		},
		{
			name:    "empty seller",
			trade:   makeTrade("t-1", "buyer", "", 100, 5),
			wantErr: true,
			errMsg:  "both buyer and seller participant IDs are required",
		},
		{
			name:    "both participants empty",
			trade:   makeTrade("t-1", "", "", 100, 5),
			wantErr: true,
			errMsg:  "both buyer and seller participant IDs are required",
		},
		{
			name:    "valid trade",
			trade:   makeTrade("t-1", "buyer", "seller", 100, 5),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewService(&seqIDGen{})
			_, err := svc.Novate(tt.trade)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.errMsg)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestNovateMultipleTrades(t *testing.T) {
	svc := NewService(&seqIDGen{})

	// Novate multiple trades to verify ID generation increments
	r1, err := svc.Novate(makeTrade("t-1", "buyer-1", "seller-1", 500, 10))
	if err != nil {
		t.Fatalf("trade 1: %v", err)
	}
	r2, err := svc.Novate(makeTrade("t-2", "buyer-2", "seller-2", 600, 20))
	if err != nil {
		t.Fatalf("trade 2: %v", err)
	}

	// All four obligation IDs must be unique
	ids := map[string]bool{
		r1.BuyerObligation.ObligationID:  true,
		r1.SellerObligation.ObligationID: true,
		r2.BuyerObligation.ObligationID:  true,
		r2.SellerObligation.ObligationID: true,
	}
	if len(ids) != 4 {
		t.Error("expected 4 unique obligation IDs across 2 novations")
	}
}

func TestNovateValueCalculation(t *testing.T) {
	tests := []struct {
		name      string
		price     int64
		qty       uint64
		wantValue string
	}{
		{"small trade", 100, 1, "100"},
		{"standard trade", 500, 10, "5000"},
		{"large quantity", 1000, 1000, "1000000"},
		{"unit price", 1, 100, "100"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewService(&seqIDGen{})
			r, err := svc.Novate(makeTrade("t-1", "b", "s", tt.price, tt.qty))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if r.BuyerObligation.Value.String() != tt.wantValue {
				t.Errorf("buyer value = %s, want %s", r.BuyerObligation.Value.String(), tt.wantValue)
			}
			if r.SellerObligation.Value.String() != tt.wantValue {
				t.Errorf("seller value = %s, want %s", r.SellerObligation.Value.String(), tt.wantValue)
			}
		})
	}
}

func TestNovateObligationFields(t *testing.T) {
	svc := NewService(&seqIDGen{})
	trade := makeTradeWithInstrument("t-99", "ACME", "GLOBEX", "CORN-2026M12", 750, 25)

	r, err := svc.Novate(trade)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Buyer obligation fields
	bo := r.BuyerObligation
	if bo.TradeID != "t-99" {
		t.Errorf("buyer tradeID = %s, want t-99", bo.TradeID)
	}
	if bo.InstrumentID != "CORN-2026M12" {
		t.Errorf("buyer instrument = %s, want CORN-2026M12", bo.InstrumentID)
	}
	if bo.ParticipantID != "ACME" {
		t.Errorf("buyer participant = %s, want ACME", bo.ParticipantID)
	}
	if bo.Side != types.SideBuy {
		t.Errorf("buyer side = %v, want BUY", bo.Side)
	}
	if bo.Quantity != 25 {
		t.Errorf("buyer qty = %d, want 25", bo.Quantity)
	}
	if bo.Price.String() != "750" {
		t.Errorf("buyer price = %s, want 750", bo.Price.String())
	}
	if bo.Status != types.ClearingStatusNovated {
		t.Errorf("buyer status = %v, want NOVATED", bo.Status)
	}
	if bo.CreatedAt.IsZero() {
		t.Error("buyer CreatedAt should not be zero")
	}
	if bo.NovatedAt.IsZero() {
		t.Error("buyer NovatedAt should not be zero")
	}

	// Seller obligation fields
	so := r.SellerObligation
	if so.ParticipantID != "GLOBEX" {
		t.Errorf("seller participant = %s, want GLOBEX", so.ParticipantID)
	}
	if so.Side != types.SideSell {
		t.Errorf("seller side = %v, want SELL", so.Side)
	}
	if so.InstrumentID != "CORN-2026M12" {
		t.Errorf("seller instrument = %s, want CORN-2026M12", so.InstrumentID)
	}
}

func TestNovateWithDecimalPrice(t *testing.T) {
	svc := NewService(&seqIDGen{})

	trade := types.Trade{
		TradeID:             "t-dec",
		InstrumentID:        "WHT-HRW-2026M07",
		BuyOrderID:          "buy-1",
		SellOrderID:         "sell-1",
		BuyerParticipantID:  "buyer",
		SellerParticipantID: "seller",
		Price:               types.NewDecimal(123, 4500), // 123.45
		Quantity:            4,
		AggressorSide:       types.SideBuy,
		ExecutedAt:          time.Now(),
	}

	r, err := svc.Novate(trade)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Value = 123.45 * 4 = 493.80
	expectedValue := types.NewDecimal(123, 4500).MulUint64(4)
	if !r.BuyerObligation.Value.Equal(expectedValue) {
		t.Errorf("value = %s, want %s", r.BuyerObligation.Value.String(), expectedValue.String())
	}
}

func TestNovateTimestamps(t *testing.T) {
	svc := NewService(&seqIDGen{})
	before := time.Now()
	r, err := svc.Novate(makeTrade("t-ts", "b", "s", 100, 1))
	after := time.Now()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// CreatedAt and NovatedAt should be between before and after
	for _, obl := range []types.ClearingObligation{r.BuyerObligation, r.SellerObligation} {
		if obl.CreatedAt.Before(before) || obl.CreatedAt.After(after) {
			t.Errorf("CreatedAt %v not between %v and %v", obl.CreatedAt, before, after)
		}
		if obl.NovatedAt.Before(before) || obl.NovatedAt.After(after) {
			t.Errorf("NovatedAt %v not between %v and %v", obl.NovatedAt, before, after)
		}
	}
}

// Test types.Side methods exercised through novation context
func TestSideStringAndOpposite(t *testing.T) {
	if types.SideBuy.String() != "BUY" {
		t.Errorf("SideBuy.String() = %s, want BUY", types.SideBuy.String())
	}
	if types.SideSell.String() != "SELL" {
		t.Errorf("SideSell.String() = %s, want SELL", types.SideSell.String())
	}
	if types.SideBuy.Opposite() != types.SideSell {
		t.Error("SideBuy.Opposite() should be SideSell")
	}
	if types.SideSell.Opposite() != types.SideBuy {
		t.Error("SideSell.Opposite() should be SideBuy")
	}
}

func TestClearingStatusString(t *testing.T) {
	tests := []struct {
		status types.ClearingStatus
		want   string
	}{
		{types.ClearingStatusPending, "PENDING"},
		{types.ClearingStatusNovated, "NOVATED"},
		{types.ClearingStatusNetted, "NETTED"},
		{types.ClearingStatusSettled, "SETTLED"},
		{types.ClearingStatusRejected, "REJECTED"},
		{types.ClearingStatus(99), "UNKNOWN"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.status.String(); got != tt.want {
				t.Errorf("ClearingStatus(%d).String() = %s, want %s", tt.status, got, tt.want)
			}
		})
	}
}

func TestPositionPredicates(t *testing.T) {
	long := types.Position{NetQuantity: 10}
	if !long.IsLong() {
		t.Error("expected IsLong true")
	}
	if long.IsShort() {
		t.Error("expected IsShort false")
	}
	if long.IsFlat() {
		t.Error("expected IsFlat false")
	}

	short := types.Position{NetQuantity: -5}
	if short.IsLong() {
		t.Error("expected IsLong false")
	}
	if !short.IsShort() {
		t.Error("expected IsShort true")
	}

	flat := types.Position{NetQuantity: 0}
	if !flat.IsFlat() {
		t.Error("expected IsFlat true")
	}
}

func TestDecimalParseAndString(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"100", "100"},
		{"100.50", "100.5"},
		{"0", "0"},
		{"-50", "-50"},
		{"-123.4567", "-123.4567"},
		{"0.0001", "0.0001"},
		{"999.99", "999.99"},
		{"  42  ", "42"},
		{"", "0"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			d, err := types.ParseDecimal(tt.input)
			if err != nil {
				t.Fatalf("ParseDecimal(%q) error: %v", tt.input, err)
			}
			if d.String() != tt.want {
				t.Errorf("ParseDecimal(%q).String() = %s, want %s", tt.input, d.String(), tt.want)
			}
		})
	}
}

func TestDecimalParseErrors(t *testing.T) {
	tests := []string{
		"abc",
		"12.34.56",
		"1.abcd",
	}
	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			_, err := types.ParseDecimal(input)
			if err == nil {
				t.Errorf("expected error for ParseDecimal(%q)", input)
			}
		})
	}
}

func TestDecimalParseTruncation(t *testing.T) {
	// Input with >4 decimal places should truncate
	d, err := types.ParseDecimal("1.123456789")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.String() != "1.1234" {
		t.Errorf("got %s, want 1.1234", d.String())
	}
}

func TestDecimalComparison(t *testing.T) {
	a := types.DecimalFromInt(100)
	b := types.DecimalFromInt(200)
	c := types.DecimalFromInt(100)

	if !a.LessThan(b) {
		t.Error("100 should be less than 200")
	}
	if !b.GreaterThan(a) {
		t.Error("200 should be greater than 100")
	}
	if !a.Equal(c) {
		t.Error("100 should equal 100")
	}

	if a.Cmp(b) != -1 {
		t.Errorf("Cmp(100, 200) = %d, want -1", a.Cmp(b))
	}
	if b.Cmp(a) != 1 {
		t.Errorf("Cmp(200, 100) = %d, want 1", b.Cmp(a))
	}
	if a.Cmp(c) != 0 {
		t.Errorf("Cmp(100, 100) = %d, want 0", a.Cmp(c))
	}
}

func TestDecimalAbs(t *testing.T) {
	pos := types.DecimalFromInt(50)
	neg := types.DecimalFromInt(-50)
	zero := types.DecimalZero()

	if !pos.Abs().Equal(types.DecimalFromInt(50)) {
		t.Error("Abs(50) should be 50")
	}
	if !neg.Abs().Equal(types.DecimalFromInt(50)) {
		t.Error("Abs(-50) should be 50")
	}
	if !zero.Abs().IsZero() {
		t.Error("Abs(0) should be 0")
	}
}

func TestDecimalIsZeroAndIsNeg(t *testing.T) {
	if !types.DecimalZero().IsZero() {
		t.Error("DecimalZero should be zero")
	}
	if types.DecimalFromInt(1).IsZero() {
		t.Error("1 should not be zero")
	}
	if !types.DecimalFromInt(-1).IsNeg() {
		t.Error("-1 should be negative")
	}
	if types.DecimalFromInt(1).IsNeg() {
		t.Error("1 should not be negative")
	}
}

func TestDecimalStringNegative(t *testing.T) {
	d := types.NewDecimal(-5, 0) // This won't work as expected, use Negate
	// Use negate for negative with fraction
	d2 := types.NewDecimal(5, 2500).Negate() // -5.25
	if d2.String() != "-5.25" {
		t.Errorf("got %s, want -5.25", d2.String())
	}

	// Zero fraction negative
	d3 := types.DecimalFromInt(-100)
	if d3.String() != "-100" {
		t.Errorf("got %s, want -100", d3.String())
	}

	_ = d // use d to avoid unused var
}

func TestNewDecimal(t *testing.T) {
	d := types.NewDecimal(5, 2500) // 5.25
	if d.String() != "5.25" {
		t.Errorf("NewDecimal(5, 2500) = %s, want 5.25", d.String())
	}
}

func TestNettingEfficiencyViaTypes(t *testing.T) {
	tests := []struct {
		name     string
		longQty  uint64
		shortQty uint64
		netQty   int64
		wantEff  float64 // approximate
	}{
		{"full offset", 100, 100, 0, 100.0},
		{"no offset", 100, 0, 100, 0.0},
		{"partial offset", 100, 80, 20, 88.89},
		{"zero gross", 0, 0, 0, 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := types.NettingResult{
				GrossLongQty:  tt.longQty,
				GrossShortQty: tt.shortQty,
				NetQuantity:   tt.netQty,
			}
			eff := r.NettingEfficiency()
			if tt.wantEff == 0 && eff != 0 {
				t.Errorf("efficiency = %.2f, want 0", eff)
			} else if tt.wantEff == 100 && eff != 100 {
				t.Errorf("efficiency = %.2f, want 100", eff)
			} else if tt.wantEff > 0 && tt.wantEff < 100 {
				if eff < tt.wantEff-1 || eff > tt.wantEff+1 {
					t.Errorf("efficiency = %.2f, want ~%.2f", eff, tt.wantEff)
				}
			}
		})
	}
}
