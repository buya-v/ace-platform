package engine

import (
	"fmt"
	"sync"
	"time"

	"github.com/garudax-platform/margin-engine/internal/margin"
	"github.com/garudax-platform/margin-engine/internal/margincall"
	"github.com/garudax-platform/margin-engine/internal/params"
	"github.com/garudax-platform/margin-engine/internal/types"
)

// PositionSource provides positions from the clearing engine.
type PositionSource interface {
	GetPositions(participantID string) []types.Position
	GetPositionsByInstrument(instrumentID string) []types.Position
}

// CollateralSource provides collateral balances.
type CollateralSource interface {
	GetCollateral(participantID string) types.Decimal
}

// MarginHandler is called after margin is calculated for a participant.
type MarginHandler func(pm types.PortfolioMargin)

// MarginCallHandler is called when a margin call is issued.
type MarginCallHandler func(call types.MarginCall)

// Engine orchestrates margin calculation, collateral checks, and margin call generation.
type Engine struct {
	mu sync.Mutex

	calculator  *margin.Calculator
	callService *margincall.Service
	paramStore  *params.Store
	collateral  CollateralSource

	marginHandler     MarginHandler
	marginCallHandler MarginCallHandler

	// In-memory cache of latest portfolio margins
	portfolios map[string]*types.PortfolioMargin
}

// NewEngine creates a new margin engine.
func NewEngine(
	paramStore *params.Store,
	callIDGen margincall.IDGenerator,
	collateral CollateralSource,
	callDeadline time.Duration,
) *Engine {
	return &Engine{
		calculator:  margin.NewCalculator(paramStore),
		callService: margincall.NewService(callIDGen, callDeadline),
		paramStore:  paramStore,
		collateral:  collateral,
		portfolios:  make(map[string]*types.PortfolioMargin),
	}
}

// SetMarginHandler sets a callback invoked after margin calculation.
// The handler field is guarded by e.mu so it may be set concurrently with
// CalculateMargin without a data race.
func (e *Engine) SetMarginHandler(h MarginHandler) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.marginHandler = h
}

// SetMarginCallHandler sets a callback invoked when a margin call is issued.
func (e *Engine) SetMarginCallHandler(h MarginCallHandler) {
	e.callService.SetHandler(margincall.CallHandler(h))
}

// CalculateMargin computes margin for a participant given their current positions.
func (e *Engine) CalculateMargin(participantID string, positions []types.Position) (*types.PortfolioMargin, error) {
	pm, marginHandler, err := e.calculateMarginLocked(participantID, positions)
	if err != nil {
		return nil, err
	}

	// Invoke the margin handler OUTSIDE the critical section so a slow or
	// re-entrant callback cannot deadlock on e.mu or serialize calculations.
	if marginHandler != nil {
		marginHandler(pm)
	}

	// Evaluate margin call. callService has its own synchronization and fires
	// its own handler outside its lock, so this runs without e.mu held.
	if _, err := e.callService.Evaluate(pm); err != nil {
		return nil, fmt.Errorf("margin engine: margin call evaluation failed: %w", err)
	}

	return &pm, nil
}

// calculateMarginLocked computes and caches the portfolio margin under e.mu and
// returns a snapshot of the margin handler for the caller to invoke after the
// lock is released.
func (e *Engine) calculateMarginLocked(participantID string, positions []types.Position) (types.PortfolioMargin, MarginHandler, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	collateral := e.collateral.GetCollateral(participantID)

	pm, err := e.calculator.CalculatePortfolio(participantID, positions, collateral)
	if err != nil {
		return types.PortfolioMargin{}, nil, fmt.Errorf("margin engine: calculation failed for %s: %w", participantID, err)
	}

	// Cache
	stored := pm
	e.portfolios[participantID] = &stored

	return pm, e.marginHandler, nil
}

// GetPortfolioMargin returns the last calculated margin for a participant.
func (e *Engine) GetPortfolioMargin(participantID string) (types.PortfolioMargin, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()

	pm, ok := e.portfolios[participantID]
	if !ok {
		return types.PortfolioMargin{}, false
	}
	return *pm, true
}

// GetActiveMarginCall returns the active margin call for a participant.
func (e *Engine) GetActiveMarginCall(participantID string) (types.MarginCall, bool) {
	return e.callService.GetActive(participantID)
}

// GetAllActiveMarginCalls returns all active margin calls.
func (e *Engine) GetAllActiveMarginCalls() []types.MarginCall {
	return e.callService.GetAllActive()
}

// CheckDeadlines checks all active margin calls and breaches those past deadline.
func (e *Engine) CheckDeadlines(now time.Time) []types.MarginCall {
	return e.callService.CheckDeadlines(now)
}

// UpdateSpotPrice updates the mark price for an instrument. This should be called
// when market data updates arrive, triggering margin recalculation.
func (e *Engine) UpdateSpotPrice(instrumentID string, price types.Decimal) error {
	return e.paramStore.UpdateSpotPrice(instrumentID, price)
}

// GetMarginCallStats returns margin call summary statistics.
func (e *Engine) GetMarginCallStats() margincall.CallStats {
	return e.callService.Stats()
}

// GetParamStore returns the parameter store for external configuration.
func (e *Engine) GetParamStore() *params.Store {
	return e.paramStore
}
