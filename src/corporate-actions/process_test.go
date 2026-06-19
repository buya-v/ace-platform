package corporateactions

import (
	"errors"
	"testing"
)

// ----------------------------------------------------------------------------
// Dividend processing
// ----------------------------------------------------------------------------

func TestProcessDividend_HappyPath(t *testing.T) {
	res, err := ProcessDividend(dividendAction(), DividendTerms{AmountPerShare: 2}, []Position{pos("p1", 100), pos("p2", 50)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Action.Status != StatusCompleted {
		t.Fatalf("action status=%s want COMPLETED", res.Action.Status)
	}
	if len(res.Entitlements) != 2 {
		t.Fatalf("got %d entitlements want 2", len(res.Entitlements))
	}
	if res.RightsEntitlements != nil || res.AdjustedPositions != nil {
		t.Errorf("non-dividend result fields should be nil: %+v", res)
	}
	if len(res.Events) != 2 {
		t.Fatalf("got %d events want 2", len(res.Events))
	}
	if res.Events[0].Type != EventProcessing || res.Events[0].Status != StatusProcessing {
		t.Errorf("first event wrong: %+v", res.Events[0])
	}
	if res.Events[1].Type != EventCompleted || res.Events[1].Status != StatusCompleted {
		t.Errorf("second event wrong: %+v", res.Events[1])
	}
	if res.Events[1].HolderCount != 2 {
		t.Errorf("completed holder count=%d want 2", res.Events[1].HolderCount)
	}
	if res.Events[1].TenantID != tenant || res.Events[1].InstrumentID != instr || res.Events[1].CorporateActionID != "ca-1" {
		t.Errorf("event metadata not propagated: %+v", res.Events[1])
	}
}

func TestProcessDividend_RollsBackOnInvalidTerms(t *testing.T) {
	res, err := ProcessDividend(dividendAction(), DividendTerms{AmountPerShare: -1}, []Position{pos("p1", 100)})
	if !errors.Is(err, ErrNegativeDividend) {
		t.Fatalf("err=%v want ErrNegativeDividend", err)
	}
	if res.Action.Status != "" {
		t.Errorf("expected zero-value result on error, got %+v", res.Action)
	}
}

func TestProcessDividend_RejectsWrongType(t *testing.T) {
	_, err := ProcessDividend(splitAction(), DividendTerms{AmountPerShare: 1}, nil)
	if !errors.Is(err, ErrWrongActionType) {
		t.Fatalf("err=%v want ErrWrongActionType", err)
	}
}

func TestProcessDividend_RejectsNonAnnouncedState(t *testing.T) {
	ca := dividendAction()
	ca.Status = StatusCompleted
	_, err := ProcessDividend(ca, DividendTerms{AmountPerShare: 1}, nil)
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("err=%v want ErrInvalidTransition", err)
	}
}

func TestProcessDividend_RejectsMissingTenant(t *testing.T) {
	ca := dividendAction()
	ca.TenantID = ""
	_, err := ProcessDividend(ca, DividendTerms{AmountPerShare: 1}, nil)
	if !errors.Is(err, ErrMissingTenant) {
		t.Fatalf("err=%v want ErrMissingTenant", err)
	}
}

// ----------------------------------------------------------------------------
// Split processing
// ----------------------------------------------------------------------------

func TestProcessSplit_HappyPath(t *testing.T) {
	positions := []Position{
		pos("p1", 100),
		{ParticipantID: "other", InstrumentID: otherInstr, TenantID: tenant, Quantity: 100}, // ineligible
	}
	res, err := ProcessSplit(splitAction(), SplitTerms{NewShares: 2, OldShares: 1}, positions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Action.Status != StatusCompleted {
		t.Fatalf("status=%s want COMPLETED", res.Action.Status)
	}
	if len(res.AdjustedPositions) != 2 {
		t.Fatalf("got %d positions want 2", len(res.AdjustedPositions))
	}
	if res.AdjustedPositions[0].Quantity != 200 {
		t.Errorf("eligible qty=%d want 200", res.AdjustedPositions[0].Quantity)
	}
	if res.AdjustedPositions[1].Quantity != 100 {
		t.Errorf("ineligible qty=%d want 100 (unchanged)", res.AdjustedPositions[1].Quantity)
	}
	// Only the one eligible holder should be counted.
	if res.Events[1].HolderCount != 1 {
		t.Errorf("holder count=%d want 1", res.Events[1].HolderCount)
	}
}

func TestProcessSplit_RollsBackOnInvalidRatio(t *testing.T) {
	_, err := ProcessSplit(splitAction(), SplitTerms{NewShares: 0, OldShares: 1}, []Position{pos("p1", 100)})
	if !errors.Is(err, ErrInvalidRatio) {
		t.Fatalf("err=%v want ErrInvalidRatio", err)
	}
}

func TestProcessSplit_RejectsWrongType(t *testing.T) {
	_, err := ProcessSplit(dividendAction(), SplitTerms{NewShares: 2, OldShares: 1}, nil)
	if !errors.Is(err, ErrWrongActionType) {
		t.Fatalf("err=%v want ErrWrongActionType", err)
	}
}

// ----------------------------------------------------------------------------
// Rights processing
// ----------------------------------------------------------------------------

func TestProcessRights_HappyPath(t *testing.T) {
	terms := RightsTerms{NewShares: 1, OldShares: 5, SubscriptionPrice: 8}
	res, err := ProcessRights(rightsAction(), terms, []Position{pos("p1", 100)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Action.Status != StatusCompleted {
		t.Fatalf("status=%s want COMPLETED", res.Action.Status)
	}
	if len(res.RightsEntitlements) != 1 || res.RightsEntitlements[0].RightsQuantity != 20 {
		t.Fatalf("rights entitlement wrong: %+v", res.RightsEntitlements)
	}
	if res.Events[1].HolderCount != 1 {
		t.Errorf("holder count=%d want 1", res.Events[1].HolderCount)
	}
}

func TestProcessRights_RejectsWrongType(t *testing.T) {
	_, err := ProcessRights(dividendAction(), RightsTerms{NewShares: 1, OldShares: 5, SubscriptionPrice: 1}, nil)
	if !errors.Is(err, ErrWrongActionType) {
		t.Fatalf("err=%v want ErrWrongActionType", err)
	}
}

func TestProcessRights_RollsBackOnInvalidTerms(t *testing.T) {
	_, err := ProcessRights(rightsAction(), RightsTerms{NewShares: 1, OldShares: 5, SubscriptionPrice: -1}, []Position{pos("p1", 100)})
	if !errors.Is(err, ErrNegativePrice) {
		t.Fatalf("err=%v want ErrNegativePrice", err)
	}
}

// ----------------------------------------------------------------------------
// Cancellation
// ----------------------------------------------------------------------------

func TestCancel_HappyPath(t *testing.T) {
	res, err := Cancel(dividendAction())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Action.Status != StatusCancelled {
		t.Fatalf("status=%s want CANCELLED", res.Action.Status)
	}
	if len(res.Events) != 1 || res.Events[0].Type != EventCancelled {
		t.Fatalf("expected one cancelled event, got %+v", res.Events)
	}
}

func TestCancel_RejectsTerminalAction(t *testing.T) {
	ca := dividendAction()
	ca.Status = StatusCompleted
	_, err := Cancel(ca)
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("err=%v want ErrInvalidTransition", err)
	}
}

// ----------------------------------------------------------------------------
// Runtime dispatch
// ----------------------------------------------------------------------------

func TestProcess_Dispatch(t *testing.T) {
	t.Run("dividend", func(t *testing.T) {
		res, err := Process(dividendAction(), DividendTerms{AmountPerShare: 1}, []Position{pos("p1", 10)})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(res.Entitlements) != 1 {
			t.Fatalf("got %d entitlements want 1", len(res.Entitlements))
		}
	})
	t.Run("split", func(t *testing.T) {
		res, err := Process(splitAction(), SplitTerms{NewShares: 2, OldShares: 1}, []Position{pos("p1", 10)})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.AdjustedPositions[0].Quantity != 20 {
			t.Fatalf("qty=%d want 20", res.AdjustedPositions[0].Quantity)
		}
	})
	t.Run("rights", func(t *testing.T) {
		res, err := Process(rightsAction(), RightsTerms{NewShares: 1, OldShares: 5, SubscriptionPrice: 2}, []Position{pos("p1", 100)})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.RightsEntitlements[0].RightsQuantity != 20 {
			t.Fatalf("rights=%d want 20", res.RightsEntitlements[0].RightsQuantity)
		}
	})
}

func TestProcess_DispatchTermsMismatch(t *testing.T) {
	cases := []struct {
		name  string
		ca    CorporateAction
		terms any
	}{
		{"dividend with split terms", dividendAction(), SplitTerms{NewShares: 2, OldShares: 1}},
		{"split with dividend terms", splitAction(), DividendTerms{AmountPerShare: 1}},
		{"rights with split terms", rightsAction(), SplitTerms{NewShares: 1, OldShares: 1}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := Process(c.ca, c.terms, nil); !errors.Is(err, ErrWrongActionType) {
				t.Fatalf("err=%v want ErrWrongActionType", err)
			}
		})
	}
}

func TestProcess_UnsupportedActionType(t *testing.T) {
	ca := dividendAction()
	ca.ActionType = Merger
	if _, err := Process(ca, DividendTerms{AmountPerShare: 1}, nil); !errors.Is(err, ErrWrongActionType) {
		t.Fatalf("err=%v want ErrWrongActionType", err)
	}
}
