package position

import (
	"fmt"
	"sync"
	"time"

	"github.com/ace-platform/clearing-engine/internal/types"
)

// positionKey uniquely identifies a position.
type positionKey struct {
	ParticipantID string
	InstrumentID  string
}

// Manager tracks net positions per participant per instrument.
// It is the source of truth for who holds what.
type Manager struct {
	mu        sync.RWMutex
	positions map[positionKey]*types.Position
}

func NewManager() *Manager {
	return &Manager{
		positions: make(map[positionKey]*types.Position),
	}
}

// Apply updates a participant's position based on a clearing obligation.
// Buy obligations increase net quantity; sell obligations decrease it.
// When a position crosses from long to short (or vice versa), realized P&L
// is calculated on the closed portion.
func (m *Manager) Apply(obl types.ClearingObligation) (*types.Position, error) {
	if obl.ParticipantID == "" {
		return nil, fmt.Errorf("position: participant ID is required")
	}
	if obl.Quantity == 0 {
		return nil, fmt.Errorf("position: obligation quantity must be positive")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	key := positionKey{
		ParticipantID: obl.ParticipantID,
		InstrumentID:  obl.InstrumentID,
	}

	pos, exists := m.positions[key]
	if !exists {
		pos = &types.Position{
			ParticipantID: obl.ParticipantID,
			InstrumentID:  obl.InstrumentID,
			AvgEntryPrice: types.DecimalZero(),
			RealizedPnL:   types.DecimalZero(),
		}
		m.positions[key] = pos
	}

	qty := int64(obl.Quantity)
	if obl.Side == types.SideSell {
		qty = -qty
		pos.TotalSellQty += obl.Quantity
	} else {
		pos.TotalBuyQty += obl.Quantity
	}

	oldNet := pos.NetQuantity
	newNet := oldNet + qty

	// Calculate realized P&L when position is being reduced or flipped
	if oldNet != 0 && ((oldNet > 0 && qty < 0) || (oldNet < 0 && qty > 0)) {
		// Closing (or partially closing) a position
		var closedQty int64
		if abs64(qty) >= abs64(oldNet) {
			closedQty = abs64(oldNet) // Fully closed (possibly flipped)
		} else {
			closedQty = abs64(qty) // Partially closed
		}

		// P&L = (exit price - entry price) * closed qty * direction
		priceDiff := obl.Price.Sub(pos.AvgEntryPrice)
		pnl := priceDiff.MulUint64(uint64(closedQty))
		if oldNet < 0 {
			// Short position: profit when price goes down
			pnl = pnl.Negate()
		}
		pos.RealizedPnL = pos.RealizedPnL.Add(pnl)
	}

	// Update average entry price
	if newNet == 0 {
		// Flat — reset avg price
		pos.AvgEntryPrice = types.DecimalZero()
	} else if (oldNet >= 0 && qty > 0) || (oldNet <= 0 && qty < 0) {
		// Adding to position (same direction) — recalculate VWAP
		if oldNet == 0 {
			pos.AvgEntryPrice = obl.Price
		} else {
			// VWAP = (old_avg * old_qty + new_price * new_qty) / total_qty
			oldValue := pos.AvgEntryPrice.MulUint64(uint64(abs64(oldNet)))
			addedValue := obl.Price.MulUint64(obl.Quantity)
			totalValue := oldValue.Add(addedValue)
			totalQty := abs64(newNet)
			// Integer division for VWAP in fixed-point
			pos.AvgEntryPrice = types.DecimalFromRaw(totalValue.Raw() / totalQty)
		}
	} else if abs64(newNet) > 0 && ((oldNet > 0 && newNet < 0) || (oldNet < 0 && newNet > 0)) {
		// Position flipped — new avg price is the trade price for the remainder
		pos.AvgEntryPrice = obl.Price
	}
	// If partially closing (same direction remaining), avg price stays the same

	pos.NetQuantity = newNet
	pos.UpdatedAt = time.Now()

	return pos, nil
}

// Get returns the current position for a participant/instrument pair.
func (m *Manager) Get(participantID, instrumentID string) (types.Position, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	key := positionKey{ParticipantID: participantID, InstrumentID: instrumentID}
	pos, ok := m.positions[key]
	if !ok {
		return types.Position{}, false
	}
	return *pos, true
}

// GetAll returns all positions for a participant.
func (m *Manager) GetAll(participantID string) []types.Position {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []types.Position
	for k, pos := range m.positions {
		if k.ParticipantID == participantID {
			result = append(result, *pos)
		}
	}
	return result
}

// GetByInstrument returns all positions for an instrument.
func (m *Manager) GetByInstrument(instrumentID string) []types.Position {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []types.Position
	for k, pos := range m.positions {
		if k.InstrumentID == instrumentID {
			result = append(result, *pos)
		}
	}
	return result
}

func abs64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
