package engine

import (
	"fmt"
	"sync"

	"github.com/garudax-platform/clearing-engine/internal/defaultmgmt"
	"github.com/garudax-platform/clearing-engine/internal/netting"
	"github.com/garudax-platform/clearing-engine/internal/novation"
	"github.com/garudax-platform/clearing-engine/internal/position"
	"github.com/garudax-platform/clearing-engine/internal/store"
	"github.com/garudax-platform/clearing-engine/internal/types"
)

// TradeHandler is called for each cleared trade with its novation result.
type TradeHandler func(trade types.Trade, result novation.NovationResult)

// Engine is the clearing engine that coordinates novation, position management,
// and netting. It consumes trades from the matching engine and produces
// clearing obligations.
type Engine struct {
	mu sync.Mutex

	novationSvc *novation.Service
	positionMgr *position.Manager
	nettingSvc  *netting.Service
	oblStore    store.ObligationStore

	// Default fund management and waterfall
	defaultFundMgr *defaultmgmt.DefaultFundManager
	waterfall      *defaultmgmt.DefaultWaterfall

	tradeHandler TradeHandler

	// Track processed trades to ensure idempotency
	processedTrades map[string]bool
}

func NewEngine(
	idGen types.IDGenerator,
	oblStore store.ObligationStore,
) *Engine {
	fundMgr := defaultmgmt.NewDefaultFundManager()
	return &Engine{
		novationSvc:     novation.NewService(idGen),
		positionMgr:     position.NewManager(),
		nettingSvc:      netting.NewService(),
		oblStore:        oblStore,
		defaultFundMgr:  fundMgr,
		waterfall:       defaultmgmt.NewDefaultWaterfall(fundMgr),
		processedTrades: make(map[string]bool),
	}
}

// SetTradeHandler sets a callback invoked after each trade is cleared.
func (e *Engine) SetTradeHandler(h TradeHandler) {
	e.tradeHandler = h
}

// ClearTrade processes a single trade through the clearing pipeline:
// 1. Validate and check idempotency
// 2. Novate: split bilateral trade into two CCP obligations
// 3. Persist obligations
// 4. Update positions for both participants
//
// Returns the novation result and updated positions for buyer and seller.
func (e *Engine) ClearTrade(trade types.Trade) (*ClearingResult, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Idempotency check
	if e.processedTrades[trade.TradeID] {
		return nil, fmt.Errorf("clearing: trade %s already processed", trade.TradeID)
	}

	// Step 1: Novate
	novResult, err := e.novationSvc.Novate(trade)
	if err != nil {
		return nil, fmt.Errorf("clearing: novation failed: %w", err)
	}

	// Step 2: Persist obligations
	if err := e.oblStore.Append(novResult.BuyerObligation); err != nil {
		return nil, fmt.Errorf("clearing: failed to store buyer obligation: %w", err)
	}
	if err := e.oblStore.Append(novResult.SellerObligation); err != nil {
		return nil, fmt.Errorf("clearing: failed to store seller obligation: %w", err)
	}

	// Step 3: Update positions
	buyerPos, err := e.positionMgr.Apply(novResult.BuyerObligation)
	if err != nil {
		return nil, fmt.Errorf("clearing: failed to update buyer position: %w", err)
	}
	sellerPos, err := e.positionMgr.Apply(novResult.SellerObligation)
	if err != nil {
		return nil, fmt.Errorf("clearing: failed to update seller position: %w", err)
	}

	// Mark trade as processed
	e.processedTrades[trade.TradeID] = true

	result := &ClearingResult{
		Trade:            trade,
		Novation:         novResult,
		BuyerPosition:    *buyerPos,
		SellerPosition:   *sellerPos,
	}

	if e.tradeHandler != nil {
		e.tradeHandler(trade, novResult)
	}

	return result, nil
}

// NetObligations performs multilateral netting on all novated obligations.
func (e *Engine) NetObligations() []types.NettingResult {
	e.mu.Lock()
	defer e.mu.Unlock()

	obligations := e.oblStore.ByStatus(types.ClearingStatusNovated)
	return e.nettingSvc.Net(obligations)
}

// NetObligationsByInstrument performs netting for a specific instrument.
func (e *Engine) NetObligationsByInstrument(instrumentID string) []types.NettingResult {
	e.mu.Lock()
	defer e.mu.Unlock()

	obligations := e.oblStore.ByInstrument(instrumentID)
	// Filter to novated only
	var novated []types.ClearingObligation
	for _, obl := range obligations {
		if obl.Status == types.ClearingStatusNovated {
			novated = append(novated, obl)
		}
	}
	return e.nettingSvc.Net(novated)
}

// GetPosition returns a participant's position in an instrument.
func (e *Engine) GetPosition(participantID, instrumentID string) (types.Position, bool) {
	return e.positionMgr.Get(participantID, instrumentID)
}

// GetPositions returns all positions for a participant.
func (e *Engine) GetPositions(participantID string) []types.Position {
	return e.positionMgr.GetAll(participantID)
}

// GetPositionsByInstrument returns all positions for an instrument.
func (e *Engine) GetPositionsByInstrument(instrumentID string) []types.Position {
	return e.positionMgr.GetByInstrument(instrumentID)
}

// GetObligations returns all clearing obligations for a trade.
func (e *Engine) GetObligations(tradeID string) []types.ClearingObligation {
	return e.oblStore.ByTrade(tradeID)
}

// --- Default Fund Management ---

// AddDefaultFundContribution records a participant's contribution to the default fund.
func (e *Engine) AddDefaultFundContribution(participantID string, amount types.Decimal) error {
	return e.defaultFundMgr.AddContribution(participantID, amount)
}

// GetDefaultFundContribution returns a participant's default fund contribution.
func (e *Engine) GetDefaultFundContribution(participantID string) types.Decimal {
	return e.defaultFundMgr.GetContribution(participantID)
}

// GetTotalDefaultFund returns the total default fund balance.
func (e *Engine) GetTotalDefaultFund() types.Decimal {
	return e.defaultFundMgr.GetTotalFund()
}

// SetCCPSkinInTheGame sets the CCP's own capital in the waterfall.
func (e *Engine) SetCCPSkinInTheGame(amount types.Decimal) error {
	return e.defaultFundMgr.SetCCPSkinInTheGame(amount)
}

// SetCCPAdditionalCapital sets the CCP's additional capital (last waterfall layer).
func (e *Engine) SetCCPAdditionalCapital(amount types.Decimal) error {
	return e.defaultFundMgr.SetCCPAdditionalCapital(amount)
}

// ExecuteDefaultWaterfall runs the five-layer default waterfall for a defaulting participant.
func (e *Engine) ExecuteDefaultWaterfall(
	defaultingParticipantID string,
	totalLoss types.Decimal,
	defaulterMargin types.Decimal,
) (*defaultmgmt.WaterfallResult, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.waterfall.ExecuteWaterfall(defaultingParticipantID, totalLoss, defaulterMargin)
}

// ClearingResult captures the full output of clearing a single trade.
type ClearingResult struct {
	Trade          types.Trade
	Novation       novation.NovationResult
	BuyerPosition  types.Position
	SellerPosition types.Position
}
