package engine

import (
	"fmt"

	"github.com/garudax-platform/matching-engine/internal/orderbook"
	"github.com/garudax-platform/matching-engine/internal/types"
)

// AuctionType identifies which call auction is being orchestrated. Opening and
// closing auctions share the same clearing-price algorithm but sit at opposite
// ends of the trading day and enter/leave different market phases.
type AuctionType int

const (
	AuctionTypeUnspecified AuctionType = 0
	AuctionTypeOpening     AuctionType = 1
	AuctionTypeClosing     AuctionType = 2
)

func (t AuctionType) String() string {
	switch t {
	case AuctionTypeOpening:
		return "OPENING"
	case AuctionTypeClosing:
		return "CLOSING"
	default:
		return "UNSPECIFIED"
	}
}

// AuctionState is the indicative snapshot of an in-progress call auction. It
// reflects where the accumulated order book would uncross if the auction closed
// at this instant, without matching any orders. Venues publish this during the
// call period so participants can react to the developing imbalance.
type AuctionState struct {
	InstrumentID   string
	AuctionType    AuctionType
	Phase          orderbook.MarketPhase
	ReferencePrice types.Decimal

	// Crossed is true when the book has executable volume at the indicative
	// price. When false the remaining fields carry their zero values.
	Crossed          bool
	IndicativePrice  types.Decimal
	IndicativeVolume uint64
	ImbalanceSide    types.Side
	ImbalanceQty     uint64

	BidOrders int
	AskOrders int
}

// AuctionUncrossResult is the outcome of closing a call auction: the official
// clearing price, the matched volume, the generated trades and execution
// reports, and the IDs of orders fully consumed by the uncrossing.
type AuctionUncrossResult struct {
	InstrumentID     string
	AuctionType      AuctionType
	Crossed          bool
	ClearingPrice    types.Decimal
	MatchedVolume    uint64
	Trades           []types.Trade
	ExecutionReports []types.ExecutionReport
	RemovedOrderIDs  []string
}

// OpenAuction begins a call auction period for an instrument by transitioning
// its order book into the matching auction phase. During the auction the book
// accumulates limit orders without continuous matching; the equilibrium is
// computed only when CloseAuction is invoked.
//
// An opening auction must be entered from PRE_OPEN; a closing auction from
// CONTINUOUS. An invalid current phase yields an error from the underlying
// phase manager.
func (e *Engine) OpenAuction(instrumentID string, auctionType AuctionType) error {
	entry, err := e.getBook(instrumentID)
	if err != nil {
		return err
	}

	phase, err := auctionEntryPhase(auctionType)
	if err != nil {
		return err
	}

	entry.mu.Lock()
	defer entry.mu.Unlock()

	// Entering an auction phase never uncrosses, so the transition result is
	// not needed here.
	_, err = entry.phaseManager.TransitionTo(phase, entry.book)
	return err
}

// IndicativeAuction returns the current indicative uncrossing for an instrument
// that is in an auction phase, without matching any orders. It is an error to
// call this when the instrument is not in an opening or closing auction.
func (e *Engine) IndicativeAuction(instrumentID string) (AuctionState, error) {
	entry, err := e.getBook(instrumentID)
	if err != nil {
		return AuctionState{}, err
	}

	entry.mu.Lock()
	defer entry.mu.Unlock()

	phase := entry.phaseManager.CurrentPhase()
	if !phase.IsAuction() {
		return AuctionState{}, fmt.Errorf(
			"instrument %s is not in an auction phase (current: %s)", instrumentID, phase)
	}

	book := entry.book
	state := AuctionState{
		InstrumentID:   instrumentID,
		AuctionType:    phaseToAuctionType(phase),
		Phase:          phase,
		ReferencePrice: book.LastTradePrice,
		BidOrders:      countOrders(book.BidLevels()),
		AskOrders:      countOrders(book.AskLevels()),
	}

	ind := entry.auctionEngine.Indicative(book.BidLevels(), book.AskLevels(), book.LastTradePrice)
	state.Crossed = ind.Crossed
	state.IndicativePrice = ind.IndicativePrice
	state.IndicativeVolume = ind.IndicativeVolume
	state.ImbalanceSide = ind.ImbalanceSide
	state.ImbalanceQty = ind.ImbalanceQty

	return state, nil
}

// CloseAuction ends a call auction period: it runs the clearing-price
// uncrossing over the accumulated book, reconciles the book by removing
// fully-filled orders, transitions to the next market phase (CONTINUOUS after
// an opening auction, POST_CLOSE after a closing auction), and dispatches the
// resulting trades and execution reports to the registered handlers.
//
// It is an error to call this when the instrument is not in an auction phase.
func (e *Engine) CloseAuction(instrumentID string) (AuctionUncrossResult, error) {
	entry, err := e.getBook(instrumentID)
	if err != nil {
		return AuctionUncrossResult{}, err
	}

	entry.mu.Lock()

	phase := entry.phaseManager.CurrentPhase()
	if !phase.IsAuction() {
		entry.mu.Unlock()
		return AuctionUncrossResult{}, fmt.Errorf(
			"instrument %s is not in an auction phase (current: %s)", instrumentID, phase)
	}

	auctionType := phaseToAuctionType(phase)
	exitPhase, err := auctionExitPhase(auctionType)
	if err != nil {
		entry.mu.Unlock()
		return AuctionUncrossResult{}, err
	}

	transResult, err := entry.phaseManager.TransitionTo(exitPhase, entry.book)
	if err != nil {
		entry.mu.Unlock()
		return AuctionUncrossResult{}, err
	}

	out := AuctionUncrossResult{
		InstrumentID: instrumentID,
		AuctionType:  auctionType,
	}
	if transResult.AuctionResult != nil {
		out.Crossed = true
		out.ClearingPrice = transResult.AuctionResult.EquilibriumPrice
		out.Trades = transResult.AuctionResult.Trades
		out.ExecutionReports = transResult.AuctionResult.ExecutionReports
		out.MatchedVolume = totalTradeVolume(out.Trades)
		// Reconcile the book: the uncrossing filled orders in place but left
		// them on their price levels with stale aggregates.
		out.RemovedOrderIDs = entry.book.RemoveFilledOrders()
	}

	entry.mu.Unlock()

	// Dispatch outside the lock, mirroring TransitionPhase/SubmitOrder.
	if out.Crossed {
		e.dispatchResults(orderbook.MatchResult{
			Trades:           out.Trades,
			ExecutionReports: out.ExecutionReports,
		})
	}

	return out, nil
}

// auctionEntryPhase maps an auction type to the market phase that opens it.
func auctionEntryPhase(t AuctionType) (orderbook.MarketPhase, error) {
	switch t {
	case AuctionTypeOpening:
		return orderbook.PhaseOpeningAuction, nil
	case AuctionTypeClosing:
		return orderbook.PhaseClosingAuction, nil
	default:
		return 0, fmt.Errorf("unknown auction type %d", t)
	}
}

// auctionExitPhase maps an auction type to the market phase entered when it
// closes: continuous trading after the open, post-close after the close.
func auctionExitPhase(t AuctionType) (orderbook.MarketPhase, error) {
	switch t {
	case AuctionTypeOpening:
		return orderbook.PhaseContinuous, nil
	case AuctionTypeClosing:
		return orderbook.PhasePostClose, nil
	default:
		return 0, fmt.Errorf("unknown auction type %d", t)
	}
}

// phaseToAuctionType classifies an auction phase. Callers must ensure the phase
// is an auction phase; any non-closing auction phase is treated as opening.
func phaseToAuctionType(phase orderbook.MarketPhase) AuctionType {
	if phase == orderbook.PhaseClosingAuction {
		return AuctionTypeClosing
	}
	return AuctionTypeOpening
}

// totalTradeVolume sums the quantities of the supplied trades.
func totalTradeVolume(trades []types.Trade) uint64 {
	var total uint64
	for _, tr := range trades {
		total += tr.Quantity
	}
	return total
}

// countOrders counts the resting orders across the supplied price levels.
func countOrders(levels []*orderbook.PriceLevel) int {
	count := 0
	for _, level := range levels {
		count += level.Len()
	}
	return count
}
