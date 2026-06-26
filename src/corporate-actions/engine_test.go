package corporateactions

import (
	"errors"
	"testing"

	"github.com/garudax-platform/decimal"
)

const (
	tenant      = "mse-equities"
	otherTenant = "ace-commodities"
	instr       = "MSE:APU"
	otherInstr  = "MSE:GOV"
)

// dec builds a Decimal money fixture from a decimal string literal. It panics on
// malformed input, which is acceptable in tests.
func dec(s string) Decimal { return decimal.MustParse(s) }

func dividendAction() CorporateAction {
	return CorporateAction{
		ID:           "ca-1",
		TenantID:     tenant,
		InstrumentID: instr,
		ActionType:   Dividend,
		Status:       StatusAnnounced,
	}
}

func splitAction() CorporateAction {
	ca := dividendAction()
	ca.ActionType = StockSplit
	return ca
}

func rightsAction() CorporateAction {
	ca := dividendAction()
	ca.ActionType = RightsIssue
	return ca
}

func pos(participant string, qty int64) Position {
	return Position{ParticipantID: participant, InstrumentID: instr, TenantID: tenant, Quantity: qty}
}

// ----------------------------------------------------------------------------
// State machine
// ----------------------------------------------------------------------------

func TestCanTransition(t *testing.T) {
	cases := []struct {
		name string
		from Status
		to   Status
		want bool
	}{
		{"announced->processing", StatusAnnounced, StatusProcessing, true},
		{"announced->cancelled", StatusAnnounced, StatusCancelled, true},
		{"announced->completed (skip processing)", StatusAnnounced, StatusCompleted, false},
		{"announced->announced (self)", StatusAnnounced, StatusAnnounced, false},
		{"processing->completed", StatusProcessing, StatusCompleted, true},
		{"processing->announced (rollback)", StatusProcessing, StatusAnnounced, true},
		{"processing->cancelled", StatusProcessing, StatusCancelled, false},
		{"completed->processing (terminal)", StatusCompleted, StatusProcessing, false},
		{"completed->announced (terminal)", StatusCompleted, StatusAnnounced, false},
		{"cancelled->processing (terminal)", StatusCancelled, StatusProcessing, false},
		{"cancelled->announced (terminal)", StatusCancelled, StatusAnnounced, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := CanTransition(c.from, c.to); got != c.want {
				t.Fatalf("CanTransition(%s,%s)=%v want %v", c.from, c.to, got, c.want)
			}
		})
	}
}

func TestIsTerminal(t *testing.T) {
	cases := map[Status]bool{
		StatusAnnounced:  false,
		StatusProcessing: false,
		StatusCompleted:  true,
		StatusCancelled:  true,
	}
	for s, want := range cases {
		if got := IsTerminal(s); got != want {
			t.Errorf("IsTerminal(%s)=%v want %v", s, got, want)
		}
	}
}

func TestTransitionMutatesOnSuccess(t *testing.T) {
	ca := dividendAction() // ANNOUNCED
	if err := ca.Transition(StatusProcessing); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ca.Status != StatusProcessing {
		t.Fatalf("status=%s want PROCESSING", ca.Status)
	}
	if err := ca.Transition(StatusCompleted); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ca.Status != StatusCompleted {
		t.Fatalf("status=%s want COMPLETED", ca.Status)
	}
}

func TestTransitionRejectsAndLeavesStatusUnchanged(t *testing.T) {
	ca := dividendAction() // ANNOUNCED
	err := ca.Transition(StatusCompleted)
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("err=%v want ErrInvalidTransition", err)
	}
	if ca.Status != StatusAnnounced {
		t.Fatalf("status mutated to %s, want unchanged ANNOUNCED", ca.Status)
	}
}

func TestTransitionRollbackFromProcessing(t *testing.T) {
	ca := dividendAction()
	_ = ca.Transition(StatusProcessing)
	if err := ca.Transition(StatusAnnounced); err != nil {
		t.Fatalf("rollback should be legal: %v", err)
	}
	if ca.Status != StatusAnnounced {
		t.Fatalf("status=%s want ANNOUNCED after rollback", ca.Status)
	}
}

func TestTerminalStatesAreFinal(t *testing.T) {
	for _, term := range []Status{StatusCompleted, StatusCancelled} {
		ca := CorporateAction{Status: term}
		for _, to := range []Status{StatusAnnounced, StatusProcessing, StatusCompleted, StatusCancelled} {
			if err := ca.Transition(to); !errors.Is(err, ErrInvalidTransition) {
				t.Errorf("from terminal %s ->%s expected ErrInvalidTransition, got %v", term, to, err)
			}
		}
	}
}

// ----------------------------------------------------------------------------
// Dividend calculation
// ----------------------------------------------------------------------------

func TestCalculateDividend_Basic(t *testing.T) {
	holders := []Position{pos("p1", 100), pos("p2", 250)}
	ents, err := CalculateDividend(dividendAction(), DividendTerms{AmountPerShare: dec("1.50")}, holders)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ents) != 2 {
		t.Fatalf("got %d entitlements, want 2", len(ents))
	}
	if !ents[0].Value.Equal(dec("150.00")) {
		t.Errorf("p1 value=%v want 150.00", ents[0].Value)
	}
	if !ents[1].Value.Equal(dec("375.00")) {
		t.Errorf("p2 value=%v want 375.00", ents[1].Value)
	}
	for _, e := range ents {
		if e.Status != EntitlementPending {
			t.Errorf("entitlement status=%s want PENDING", e.Status)
		}
		if e.TenantID != tenant || e.InstrumentID != instr || e.CorporateActionID != "ca-1" {
			t.Errorf("entitlement metadata not propagated: %+v", e)
		}
	}
}

func TestCalculateDividend_Precision(t *testing.T) {
	// 333 shares * 0.1250 = 41.6250, exact in the 4dp Decimal domain (no lossy
	// 2dp rounding — the old float round2 collapsed this to 41.63).
	ents, err := CalculateDividend(dividendAction(), DividendTerms{AmountPerShare: dec("0.1250")}, []Position{pos("p1", 333)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ents[0].Value.Equal(dec("41.6250")) {
		t.Fatalf("value=%v want 41.6250", ents[0].Value)
	}
}

func TestCalculateDividend_SkipsIneligible(t *testing.T) {
	holders := []Position{
		pos("ok", 100),
		{ParticipantID: "wrong-tenant", InstrumentID: instr, TenantID: otherTenant, Quantity: 100},
		{ParticipantID: "wrong-instr", InstrumentID: otherInstr, TenantID: tenant, Quantity: 100},
		pos("zero", 0),
		pos("negative", -50),
	}
	ents, err := CalculateDividend(dividendAction(), DividendTerms{AmountPerShare: dec("2")}, holders)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ents) != 1 {
		t.Fatalf("got %d entitlements, want 1 (only eligible holder)", len(ents))
	}
	if ents[0].ParticipantID != "ok" {
		t.Fatalf("kept wrong holder: %s", ents[0].ParticipantID)
	}
}

func TestCalculateDividend_ZeroDividendAllowed(t *testing.T) {
	ents, err := CalculateDividend(dividendAction(), DividendTerms{AmountPerShare: dec("0")}, []Position{pos("p1", 100)})
	if err != nil {
		t.Fatalf("zero dividend should be allowed: %v", err)
	}
	if !ents[0].Value.IsZero() {
		t.Fatalf("value=%v want 0", ents[0].Value)
	}
}

func TestCalculateDividend_Errors(t *testing.T) {
	cases := []struct {
		name  string
		ca    CorporateAction
		terms DividendTerms
		want  error
	}{
		{"negative dividend", dividendAction(), DividendTerms{AmountPerShare: dec("-1")}, ErrNegativeDividend},
		{"wrong action type", splitAction(), DividendTerms{AmountPerShare: dec("1")}, ErrWrongActionType},
		{"missing tenant", func() CorporateAction { c := dividendAction(); c.TenantID = ""; return c }(), DividendTerms{AmountPerShare: dec("1")}, ErrMissingTenant},
		{"missing instrument", func() CorporateAction { c := dividendAction(); c.InstrumentID = ""; return c }(), DividendTerms{AmountPerShare: dec("1")}, ErrMissingInstrument},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := CalculateDividend(c.ca, c.terms, []Position{pos("p1", 10)})
			if !errors.Is(err, c.want) {
				t.Fatalf("err=%v want %v", err, c.want)
			}
		})
	}
}

func TestCalculateDividend_NeverNil(t *testing.T) {
	ents, err := CalculateDividend(dividendAction(), DividendTerms{AmountPerShare: dec("1")}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ents == nil {
		t.Fatal("expected non-nil empty slice")
	}
	if len(ents) != 0 {
		t.Fatalf("len=%d want 0", len(ents))
	}
}

// ----------------------------------------------------------------------------
// Stock split calculation
// ----------------------------------------------------------------------------

func TestSplitAdjustedQuantity(t *testing.T) {
	cases := []struct {
		name     string
		qty      int64
		terms    SplitTerms
		wantQty  int64
		wantFrac Decimal
	}{
		{"2-for-1 forward", 100, SplitTerms{NewShares: 2, OldShares: 1}, 200, dec("0")},
		{"3-for-1 forward", 50, SplitTerms{NewShares: 3, OldShares: 1}, 150, dec("0")},
		{"1-for-10 reverse", 100, SplitTerms{NewShares: 1, OldShares: 10}, 10, dec("0")},
		{"1-for-10 reverse with remainder", 105, SplitTerms{NewShares: 1, OldShares: 10}, 10, dec("0.5")},
		{"3-for-2 fractional", 101, SplitTerms{NewShares: 3, OldShares: 2}, 151, dec("0.5")},
		{"zero holding", 0, SplitTerms{NewShares: 2, OldShares: 1}, 0, dec("0")},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotQty, gotFrac, err := SplitAdjustedQuantity(c.qty, c.terms)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotQty != c.wantQty {
				t.Errorf("qty=%d want %d", gotQty, c.wantQty)
			}
			if !gotFrac.Equal(c.wantFrac) {
				t.Errorf("frac=%v want %v", gotFrac, c.wantFrac)
			}
		})
	}
}

func TestSplitAdjustedQuantity_InvalidRatio(t *testing.T) {
	for _, terms := range []SplitTerms{
		{NewShares: 0, OldShares: 1},
		{NewShares: 2, OldShares: 0},
		{NewShares: -2, OldShares: 1},
	} {
		if _, _, err := SplitAdjustedQuantity(100, terms); !errors.Is(err, ErrInvalidRatio) {
			t.Errorf("terms %+v: err=%v want ErrInvalidRatio", terms, err)
		}
	}
}

func TestSplitAdjustedPrice(t *testing.T) {
	cases := []struct {
		name  string
		price Decimal
		terms SplitTerms
		want  Decimal
	}{
		{"2-for-1 halves price", dec("100"), SplitTerms{NewShares: 2, OldShares: 1}, dec("50")},
		{"1-for-10 reverse tenx price", dec("5"), SplitTerms{NewShares: 1, OldShares: 10}, dec("50")},
		{"3-for-2 price", dec("30"), SplitTerms{NewShares: 3, OldShares: 2}, dec("20")},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := SplitAdjustedPrice(c.price, c.terms)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !got.Equal(c.want) {
				t.Fatalf("price=%v want %v", got, c.want)
			}
		})
	}
}

func TestSplitAdjustedPrice_Errors(t *testing.T) {
	if _, err := SplitAdjustedPrice(dec("-1"), SplitTerms{NewShares: 2, OldShares: 1}); !errors.Is(err, ErrNegativePrice) {
		t.Errorf("negative price: err=%v want ErrNegativePrice", err)
	}
	if _, err := SplitAdjustedPrice(dec("10"), SplitTerms{NewShares: 0, OldShares: 1}); !errors.Is(err, ErrInvalidRatio) {
		t.Errorf("invalid ratio: err=%v want ErrInvalidRatio", err)
	}
}

// SplitAdjustedPrice * SplitAdjustedQuantity should preserve total market value
// for a clean (non-fractional) split.
func TestSplit_PreservesMarketValue(t *testing.T) {
	terms := SplitTerms{NewShares: 2, OldShares: 1}
	const qty int64 = 100
	price := dec("80")
	newQty, _, _ := SplitAdjustedQuantity(qty, terms)
	newPrice, _ := SplitAdjustedPrice(price, terms)
	before := price.MulInt64(qty)
	after := newPrice.MulInt64(newQty)
	if !before.Equal(after) {
		t.Fatalf("value not preserved: before=%v after=%v", before, after)
	}
}

func TestApplySplit(t *testing.T) {
	positions := []Position{
		pos("p1", 100),
		pos("p2", 105),
		{ParticipantID: "other", InstrumentID: otherInstr, TenantID: tenant, Quantity: 100}, // untouched
	}
	out, err := ApplySplit(splitAction(), SplitTerms{NewShares: 2, OldShares: 1}, positions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 3 {
		t.Fatalf("got %d positions want 3", len(out))
	}
	if out[0].Quantity != 200 || out[1].Quantity != 210 {
		t.Errorf("split quantities wrong: %d, %d", out[0].Quantity, out[1].Quantity)
	}
	if out[2].Quantity != 100 {
		t.Errorf("ineligible position mutated: %d want 100", out[2].Quantity)
	}
	// Input slice must not be mutated.
	if positions[0].Quantity != 100 {
		t.Errorf("input position mutated: %d want 100", positions[0].Quantity)
	}
}

func TestApplySplit_Errors(t *testing.T) {
	if _, err := ApplySplit(dividendAction(), SplitTerms{NewShares: 2, OldShares: 1}, nil); !errors.Is(err, ErrWrongActionType) {
		t.Errorf("wrong action type: err=%v", err)
	}
	if _, err := ApplySplit(splitAction(), SplitTerms{NewShares: 0, OldShares: 1}, nil); !errors.Is(err, ErrInvalidRatio) {
		t.Errorf("invalid ratio: err=%v", err)
	}
}

// ----------------------------------------------------------------------------
// Rights issue calculation
// ----------------------------------------------------------------------------

func TestCalculateRights_Basic(t *testing.T) {
	// 1-for-5 at 8.00: 100 held -> 20 rights, cost 160.00.
	terms := RightsTerms{NewShares: 1, OldShares: 5, SubscriptionPrice: dec("8.00")}
	ents, err := CalculateRights(rightsAction(), terms, []Position{pos("p1", 100)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ents) != 1 {
		t.Fatalf("got %d want 1", len(ents))
	}
	e := ents[0]
	if e.RightsQuantity != 20 {
		t.Errorf("rights=%d want 20", e.RightsQuantity)
	}
	if !e.SubscriptionCost.Equal(dec("160.00")) {
		t.Errorf("cost=%v want 160.00", e.SubscriptionCost)
	}
	if e.HeldQuantity != 100 || e.Status != EntitlementPending {
		t.Errorf("metadata wrong: %+v", e)
	}
}

func TestCalculateRights_FloorsFractionalRights(t *testing.T) {
	// 1-for-3: 100 held -> floor(33.33) = 33 rights.
	terms := RightsTerms{NewShares: 1, OldShares: 3, SubscriptionPrice: dec("10")}
	ents, err := CalculateRights(rightsAction(), terms, []Position{pos("p1", 100)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ents[0].RightsQuantity != 33 {
		t.Fatalf("rights=%d want 33", ents[0].RightsQuantity)
	}
	if !ents[0].SubscriptionCost.Equal(dec("330")) {
		t.Fatalf("cost=%v want 330", ents[0].SubscriptionCost)
	}
}

func TestCalculateRights_SkipsIneligible(t *testing.T) {
	holders := []Position{
		pos("ok", 50),
		{ParticipantID: "wrong-tenant", InstrumentID: instr, TenantID: otherTenant, Quantity: 50},
		pos("zero", 0),
	}
	ents, err := CalculateRights(rightsAction(), RightsTerms{NewShares: 1, OldShares: 5, SubscriptionPrice: dec("1")}, holders)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ents) != 1 || ents[0].ParticipantID != "ok" {
		t.Fatalf("eligibility filter failed: %+v", ents)
	}
}

func TestCalculateRights_Errors(t *testing.T) {
	cases := []struct {
		name  string
		ca    CorporateAction
		terms RightsTerms
		want  error
	}{
		{"wrong action type", dividendAction(), RightsTerms{NewShares: 1, OldShares: 5, SubscriptionPrice: dec("1")}, ErrWrongActionType},
		{"zero new shares", rightsAction(), RightsTerms{NewShares: 0, OldShares: 5, SubscriptionPrice: dec("1")}, ErrInvalidRatio},
		{"zero old shares", rightsAction(), RightsTerms{NewShares: 1, OldShares: 0, SubscriptionPrice: dec("1")}, ErrInvalidRatio},
		{"negative price", rightsAction(), RightsTerms{NewShares: 1, OldShares: 5, SubscriptionPrice: dec("-1")}, ErrNegativePrice},
		{"missing tenant", func() CorporateAction { c := rightsAction(); c.TenantID = ""; return c }(), RightsTerms{NewShares: 1, OldShares: 5, SubscriptionPrice: dec("1")}, ErrMissingTenant},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := CalculateRights(c.ca, c.terms, []Position{pos("p1", 10)})
			if !errors.Is(err, c.want) {
				t.Fatalf("err=%v want %v", err, c.want)
			}
		})
	}
}

func TestTheoreticalExRightsPrice(t *testing.T) {
	// 1-for-5 at 8.00, cum price 10.00:
	// TERP = (5*10 + 1*8) / 6 = 58/6 = 9.6666... -> 9.6667 (half-even at 4dp).
	terp, err := TheoreticalExRightsPrice(dec("10.00"), RightsTerms{NewShares: 1, OldShares: 5, SubscriptionPrice: dec("8.00")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !terp.Equal(dec("9.6667")) {
		t.Fatalf("terp=%v want 9.6667", terp)
	}
}

func TestTheoreticalExRightsPrice_Errors(t *testing.T) {
	if _, err := TheoreticalExRightsPrice(dec("-1"), RightsTerms{NewShares: 1, OldShares: 5}); !errors.Is(err, ErrNegativePrice) {
		t.Errorf("negative cum price: err=%v", err)
	}
	if _, err := TheoreticalExRightsPrice(dec("10"), RightsTerms{NewShares: 0, OldShares: 5}); !errors.Is(err, ErrInvalidRatio) {
		t.Errorf("invalid ratio: err=%v", err)
	}
}

func TestCalculateRights_NeverNil(t *testing.T) {
	ents, err := CalculateRights(rightsAction(), RightsTerms{NewShares: 1, OldShares: 5, SubscriptionPrice: dec("1")}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ents == nil || len(ents) != 0 {
		t.Fatalf("expected non-nil empty slice, got %v", ents)
	}
}
