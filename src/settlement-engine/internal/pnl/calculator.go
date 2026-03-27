package pnl

import (
	"fmt"
	"time"

	"github.com/ace-platform/settlement-engine/internal/types"
	"github.com/ace-platform/settlement-engine/internal/valuation"
)

// Calculator computes daily mark-to-market P&L (variation margin) for positions.
type Calculator struct {
	priceStore *valuation.Store
}

func NewCalculator(priceStore *valuation.Store) *Calculator {
	return &Calculator{priceStore: priceStore}
}

// CalculateDaily computes the variation margin for a position on a settlement date.
//
// For existing positions: variation = (settlementPrice - previousSettlementPrice) * netQuantity
// For new positions (no previous price): variation = (settlementPrice - avgEntryPrice) * netQuantity
//
// Long positions profit when price rises; short positions profit when price falls.
func (c *Calculator) CalculateDaily(pos types.Position, settleDate time.Time) (types.PnLRecord, error) {
	sp, err := c.priceStore.GetSettlementPrice(pos.InstrumentID, settleDate)
	if err != nil {
		return types.PnLRecord{}, fmt.Errorf("pnl: %w", err)
	}

	if pos.NetQuantity == 0 {
		return types.PnLRecord{
			ParticipantID:   pos.ParticipantID,
			InstrumentID:    pos.InstrumentID,
			NetQuantity:     0,
			PreviousPrice:   sp.PreviousPrice,
			CurrentPrice:    sp.SettlementPrice,
			VariationMargin: types.DecimalZero(),
			CalculatedAt:    time.Now(),
		}, nil
	}

	// Determine reference price
	var refPrice types.Decimal
	if !sp.PreviousPrice.IsZero() {
		refPrice = sp.PreviousPrice
	} else {
		// New position — use entry price as reference
		refPrice = pos.AvgEntryPrice
	}

	// Variation margin = (current - reference) * netQuantity
	// Positive netQuantity (long) profits when price rises
	// Negative netQuantity (short) profits when price falls
	priceDiff := sp.SettlementPrice.Sub(refPrice)
	variation := priceDiff.MulInt64(pos.NetQuantity)

	return types.PnLRecord{
		ParticipantID:   pos.ParticipantID,
		InstrumentID:    pos.InstrumentID,
		NetQuantity:     pos.NetQuantity,
		PreviousPrice:   refPrice,
		CurrentPrice:    sp.SettlementPrice,
		VariationMargin: variation,
		CalculatedAt:    time.Now(),
	}, nil
}

// CalculateBatch computes variation margin for multiple positions.
func (c *Calculator) CalculateBatch(positions []types.Position, settleDate time.Time) ([]types.PnLRecord, error) {
	records := make([]types.PnLRecord, 0, len(positions))
	for _, pos := range positions {
		rec, err := c.CalculateDaily(pos, settleDate)
		if err != nil {
			return nil, fmt.Errorf("pnl: batch calculation failed for %s/%s: %w",
				pos.ParticipantID, pos.InstrumentID, err)
		}
		records = append(records, rec)
	}
	return records, nil
}

// NetByParticipant aggregates P&L records by participant, returning the net amount
// each participant owes or is owed.
func NetByParticipant(records []types.PnLRecord) map[string]types.Decimal {
	nets := make(map[string]types.Decimal)
	for _, r := range records {
		current, ok := nets[r.ParticipantID]
		if !ok {
			current = types.DecimalZero()
		}
		nets[r.ParticipantID] = current.Add(r.VariationMargin)
	}
	return nets
}
