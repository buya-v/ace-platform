// Package settlement implements the T+2 settlement engine for the securities-service.
package settlement

import (
	"crypto/rand"
	"fmt"
	"time"

	"github.com/garudax-platform/decimal"
	"github.com/garudax-platform/securities-service/internal/store"
	"github.com/garudax-platform/securities-service/internal/types"
)

// SettlementEngine processes settlement obligations through the T+2 lifecycle.
type SettlementEngine struct {
	orderStore      store.OrderStore
	settlementStore store.SettlementStore
	bondStore       store.BondStore
}

// SetBondStore configures the bond store used for accrued interest calculations.
func (e *SettlementEngine) SetBondStore(bs store.BondStore) {
	e.bondStore = bs
}

// NewSettlementEngine creates a new SettlementEngine with the given stores.
func NewSettlementEngine(
	orderStore store.OrderStore,
	settlementStore store.SettlementStore,
) *SettlementEngine {
	return &SettlementEngine{
		orderStore:      orderStore,
		settlementStore: settlementStore,
	}
}

// newUUID generates a random UUID v4 string using crypto/rand.
func newUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

// CreateObligationsFromTrades creates settlement obligations for each trade.
// For each trade it looks up the buy and sell orders to determine participant IDs,
// then creates a SETTLE_PENDING obligation with NetAmount = Price * Quantity.
func (e *SettlementEngine) CreateObligationsFromTrades(trades []types.SecurityTrade) error {
	now := time.Now().UTC().Format(time.RFC3339)

	for _, trade := range trades {
		// Look up buy order to get buyer participant ID.
		buyOrder, err := e.orderStore.Get(trade.BuyOrderID)
		if err != nil {
			return fmt.Errorf("settlement: failed to get buy order %s: %w", trade.BuyOrderID, err)
		}
		// Look up sell order to get seller participant ID.
		sellOrder, err := e.orderStore.Get(trade.SellOrderID)
		if err != nil {
			return fmt.Errorf("settlement: failed to get sell order %s: %w", trade.SellOrderID, err)
		}

		id, err := newUUID()
		if err != nil {
			return fmt.Errorf("settlement: failed to generate obligation ID: %w", err)
		}

		obligation := &types.SettlementObligation{
			ID:                  id,
			TradeID:             trade.ID,
			InstrumentID:        trade.InstrumentID,
			BuyerParticipantID:  buyOrder.ParticipantID,
			SellerParticipantID: sellOrder.ParticipantID,
			Quantity:            trade.Quantity,
			Price:               trade.Price,
			NetAmount:           trade.Price.MulInt64(int64(trade.Quantity)),
			SettlementDate:      trade.SettlementDate,
			Status:              types.SettlePending,
			CreatedAt:           now,
			UpdatedAt:           now,
		}

		if err := e.settlementStore.Create(obligation); err != nil {
			return fmt.Errorf("settlement: failed to create obligation for trade %s: %w", trade.ID, err)
		}
	}

	return nil
}

// nettingKey groups obligations by instrument + buyer + seller for netting.
type nettingKey struct {
	InstrumentID        string
	BuyerParticipantID  string
	SellerParticipantID string
}

// ProcessSettlementCycle runs the settlement cycle for a given date:
//  1. List all obligations for the date
//  2. Affirm PENDING obligations
//  3. Net AFFIRMED obligations by (instrument, buyer, seller)
//  4. Mark NETTED obligations as SETTLED (simplified, no real CSD)
//  5. Return summary counts
func (e *SettlementEngine) ProcessSettlementCycle(date string) (*types.SettlementResult, error) {
	result := &types.SettlementResult{Date: date}

	// 1. Get all obligations for this settlement date.
	obligations, err := e.settlementStore.ListByDate(date)
	if err != nil {
		return nil, fmt.Errorf("settlement cycle: failed to list obligations for %s: %w", date, err)
	}
	result.Processed = len(obligations)

	// 2. Affirm all PENDING obligations.
	for _, ob := range obligations {
		if ob.Status == types.SettlePending {
			if err := e.settlementStore.UpdateStatus(ob.ID, types.SettleAffirmed); err != nil {
				return nil, fmt.Errorf("settlement cycle: failed to affirm obligation %s: %w", ob.ID, err)
			}
			result.Affirmed++
		}
	}

	// Re-fetch after affirmation to get updated statuses.
	obligations, err = e.settlementStore.ListByDate(date)
	if err != nil {
		return nil, fmt.Errorf("settlement cycle: failed to re-list obligations for %s: %w", date, err)
	}

	// 2b. Calculate accrued interest for bond instruments.
	if e.bondStore != nil {
		settlementDate, _ := time.Parse("2006-01-02", date)
		for i := range obligations {
			ob := &obligations[i]
			bond, err := e.bondStore.Get(ob.InstrumentID)
			if err != nil {
				continue // not a bond instrument, skip
			}
			accrued := calculateAccruedInterest(bond, settlementDate, ob.Quantity)
			ob.AccruedInterest = accrued
			ob.NetAmount = ob.NetAmount.Add(accrued)
			_ = e.settlementStore.Update(ob)
		}
	}

	// 3. Group AFFIRMED by (instrument, buyer, seller) and net quantities.
	groups := make(map[nettingKey][]string) // key -> list of obligation IDs
	for _, ob := range obligations {
		if ob.Status == types.SettleAffirmed {
			key := nettingKey{
				InstrumentID:        ob.InstrumentID,
				BuyerParticipantID:  ob.BuyerParticipantID,
				SellerParticipantID: ob.SellerParticipantID,
			}
			groups[key] = append(groups[key], ob.ID)
		}
	}

	// Mark all grouped obligations as NETTED.
	for _, ids := range groups {
		for _, id := range ids {
			if err := e.settlementStore.UpdateStatus(id, types.SettleNetted); err != nil {
				return nil, fmt.Errorf("settlement cycle: failed to net obligation %s: %w", id, err)
			}
			result.Netted++
		}
	}

	// 4. Mark all NETTED obligations as SETTLED (simplified — no real CSD interaction).
	// Re-fetch to get current statuses.
	obligations, err = e.settlementStore.ListByDate(date)
	if err != nil {
		return nil, fmt.Errorf("settlement cycle: failed to re-list obligations for settlement: %w", err)
	}

	for _, ob := range obligations {
		if ob.Status == types.SettleNetted {
			if err := e.settlementStore.UpdateStatus(ob.ID, types.SettleSettled); err != nil {
				result.Failed++
				continue
			}
			result.Settled++
		}
	}

	return result, nil
}

// calculateAccruedInterest computes accrued interest for a bond obligation based on
// the bond's day count convention. For MVP, assumes 30 days since the last coupon date.
//
// CouponRate is a genuine ratio (e.g. 0.05 for 5%) and stays float64; ParValue is the
// shared decimal money type. accrued = parValue * couponRate * days * quantity / basis.
// CouponRate is converted to a Decimal at the boundary; days, quantity and basis are
// exact integers. We multiply through and divide by the basis LAST so the result keeps
// full fixed-point precision and rounds half-even only at the final money scale.
func calculateAccruedInterest(bond *types.Bond, settlementDate time.Time, quantity int) types.Decimal {
	const defaultDaysSinceLastCoupon = 30

	couponRate := bond.CouponRate // e.g. 0.05 for 5%
	parValue := bond.ParValue
	var basis int64
	switch bond.DayCountConvention {
	case types.DayCountACT360, types.DayCount30360:
		// ACT/360 and 30/360 use a 360-day basis. For MVP with 30 default days,
		// 30/360 yields months=1, remaining=0.
		basis = 360
	default:
		// ACT/365 and any unknown convention default to a 365-day basis.
		// CouponRate is already a fraction (e.g. 0.05), so we do not divide by 100.
		basis = 365
	}

	couponFactor, err := decimal.NewFromFloat(couponRate)
	if err != nil {
		return types.Decimal{}
	}

	return parValue.MulDecimal(couponFactor).
		MulInt64(defaultDaysSinceLastCoupon).
		MulInt64(int64(quantity)).
		DivInt64(basis)
}
