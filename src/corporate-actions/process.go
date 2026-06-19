package corporateactions

// This file adds the processing orchestration layer on top of the pure
// calculations in engine.go. Where engine.go answers "what is each holder
// entitled to", this file answers "how does a corporate action move through its
// lifecycle and what events does the platform emit as it does".
//
// A processing run drives a single corporate action from ANNOUNCED through
// PROCESSING to COMPLETED, runs the relevant calculation, and returns the
// generated entitlements/adjusted positions together with the lifecycle events
// that should be published to the platform event bus. If the calculation fails
// after the action has entered PROCESSING, the action is rolled back to
// ANNOUNCED (mirroring the PROCESSING -> ANNOUNCED transition) so the run leaves
// no half-processed state behind.

// EventType enumerates the lifecycle events emitted while processing an action.
// Values are namespaced so they remain unambiguous on a shared event bus.
type EventType string

const (
	EventProcessing EventType = "corporate_action.processing"
	EventCompleted  EventType = "corporate_action.completed"
	EventCancelled  EventType = "corporate_action.cancelled"
)

// Event is an emitted record of a corporate-action lifecycle milestone. It
// carries enough tenant-scoped context to be routed and audited without a
// lookup back to the originating action.
type Event struct {
	CorporateActionID string
	TenantID          string
	InstrumentID      string
	ActionType        ActionType
	Type              EventType
	Status            Status // action status at the moment the event was emitted
	HolderCount       int    // number of holders affected (0 for non-result events)
}

// ProcessResult bundles the full outcome of processing one corporate action.
// Exactly one of the typed result fields is populated depending on the action
// type; the others are nil. Events is ordered and never nil.
type ProcessResult struct {
	Action             CorporateAction     // the action with Status advanced to its terminal state
	Entitlements       []Entitlement       // cash entitlements (CA_DIVIDEND only)
	RightsEntitlements []RightsEntitlement // rights entitlements (CA_RIGHTS_ISSUE only)
	AdjustedPositions  []Position          // post-split positions (CA_STOCK_SPLIT only)
	Events             []Event             // lifecycle events, in emission order
}

// emit builds an Event describing the action at its current status.
func emit(ca CorporateAction, t EventType, holders int) Event {
	return Event{
		CorporateActionID: ca.ID,
		TenantID:          ca.TenantID,
		InstrumentID:      ca.InstrumentID,
		ActionType:        ca.ActionType,
		Type:              t,
		Status:            ca.Status,
		HolderCount:       holders,
	}
}

// begin validates the action and advances it ANNOUNCED -> PROCESSING, returning
// the PROCESSING event. The action must declare the expected type and be in a
// state from which PROCESSING is reachable, otherwise the action is left
// unchanged and the error is returned.
func begin(ca *CorporateAction, want ActionType) (Event, error) {
	if err := ca.validate(want); err != nil {
		return Event{}, err
	}
	if err := ca.Transition(StatusProcessing); err != nil {
		return Event{}, err
	}
	return emit(*ca, EventProcessing, 0), nil
}

// ProcessDividend runs the full dividend lifecycle: it advances the action to
// PROCESSING, computes one PENDING entitlement per eligible holder, then
// advances to COMPLETED and emits the processing/completed events. If the
// dividend terms are invalid the action is rolled back to ANNOUNCED.
func ProcessDividend(ca CorporateAction, terms DividendTerms, holders []Position) (ProcessResult, error) {
	startEvt, err := begin(&ca, Dividend)
	if err != nil {
		return ProcessResult{}, err
	}
	ents, err := CalculateDividend(ca, terms, holders)
	if err != nil {
		_ = ca.Transition(StatusAnnounced) // rollback; ignore error (transition is always legal here)
		return ProcessResult{}, err
	}
	_ = ca.Transition(StatusCompleted)
	return ProcessResult{
		Action:       ca,
		Entitlements: ents,
		Events:       []Event{startEvt, emit(ca, EventCompleted, len(ents))},
	}, nil
}

// ProcessSplit runs the full stock-split lifecycle: it advances the action to
// PROCESSING, adjusts eligible positions by the split ratio, then advances to
// COMPLETED. If the split ratio is invalid the action is rolled back.
func ProcessSplit(ca CorporateAction, terms SplitTerms, positions []Position) (ProcessResult, error) {
	startEvt, err := begin(&ca, StockSplit)
	if err != nil {
		return ProcessResult{}, err
	}
	adjusted, err := ApplySplit(ca, terms, positions)
	if err != nil {
		_ = ca.Transition(StatusAnnounced)
		return ProcessResult{}, err
	}
	_ = ca.Transition(StatusCompleted)
	return ProcessResult{
		Action:            ca,
		AdjustedPositions: adjusted,
		Events:            []Event{startEvt, emit(ca, EventCompleted, affectedCount(ca, positions))},
	}, nil
}

// ProcessRights runs the full rights-issue lifecycle: it advances the action to
// PROCESSING, computes one PENDING rights entitlement per eligible holder, then
// advances to COMPLETED. If the rights terms are invalid the action is rolled
// back.
func ProcessRights(ca CorporateAction, terms RightsTerms, holders []Position) (ProcessResult, error) {
	startEvt, err := begin(&ca, RightsIssue)
	if err != nil {
		return ProcessResult{}, err
	}
	ents, err := CalculateRights(ca, terms, holders)
	if err != nil {
		_ = ca.Transition(StatusAnnounced)
		return ProcessResult{}, err
	}
	_ = ca.Transition(StatusCompleted)
	return ProcessResult{
		Action:             ca,
		RightsEntitlements: ents,
		Events:             []Event{startEvt, emit(ca, EventCompleted, len(ents))},
	}, nil
}

// Cancel moves an announced action to CANCELLED and emits a cancellation event.
// Only an action that is still ANNOUNCED may be cancelled; otherwise the action
// is left unchanged and ErrInvalidTransition is returned.
func Cancel(ca CorporateAction) (ProcessResult, error) {
	if err := ca.Transition(StatusCancelled); err != nil {
		return ProcessResult{}, err
	}
	return ProcessResult{
		Action: ca,
		Events: []Event{emit(ca, EventCancelled, 0)},
	}, nil
}

// Process dispatches to the lifecycle handler matching the action's declared
// type. The terms argument must be the concrete terms value for that type
// (DividendTerms, SplitTerms, or RightsTerms); a mismatched terms type yields
// ErrWrongActionType. This is the single entry point a caller uses when the
// action type is only known at runtime.
func Process(ca CorporateAction, terms any, holders []Position) (ProcessResult, error) {
	switch ca.ActionType {
	case Dividend:
		t, ok := terms.(DividendTerms)
		if !ok {
			return ProcessResult{}, ErrWrongActionType
		}
		return ProcessDividend(ca, t, holders)
	case StockSplit:
		t, ok := terms.(SplitTerms)
		if !ok {
			return ProcessResult{}, ErrWrongActionType
		}
		return ProcessSplit(ca, t, holders)
	case RightsIssue:
		t, ok := terms.(RightsTerms)
		if !ok {
			return ProcessResult{}, ErrWrongActionType
		}
		return ProcessRights(ca, t, holders)
	default:
		// Merger and any other declared-but-unprocessed type land here.
		return ProcessResult{}, ErrWrongActionType
	}
}

// affectedCount reports how many of the supplied positions are eligible for the
// action, i.e. how many were actually adjusted by a split.
func affectedCount(ca CorporateAction, positions []Position) int {
	n := 0
	for _, p := range positions {
		if eligible(ca, p) {
			n++
		}
	}
	return n
}
