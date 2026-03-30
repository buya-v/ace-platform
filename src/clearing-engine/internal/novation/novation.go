package novation

import (
	"fmt"
	"time"

	"github.com/garudax-platform/clearing-engine/internal/types"
)

// CCP is the central counterparty identifier used in novation.
const CCP = "GarudaX-CCP"

// Service performs trade novation — replacing bilateral trades with
// two CCP-intermediated clearing obligations.
type Service struct {
	idGen types.IDGenerator
}

func NewService(idGen types.IDGenerator) *Service {
	return &Service{idGen: idGen}
}

// NovationResult contains the two obligations created from novating a trade.
type NovationResult struct {
	BuyerObligation  types.ClearingObligation
	SellerObligation types.ClearingObligation
}

// Novate takes a bilateral trade and creates two clearing obligations:
//   - Buyer ↔ CCP (buyer buys from CCP)
//   - Seller ↔ CCP (seller sells to CCP)
//
// This is the fundamental operation of central clearing — the CCP becomes
// the buyer to every seller and the seller to every buyer.
func (s *Service) Novate(trade types.Trade) (NovationResult, error) {
	if trade.TradeID == "" {
		return NovationResult{}, fmt.Errorf("novation: trade ID is required")
	}
	if trade.Quantity == 0 {
		return NovationResult{}, fmt.Errorf("novation: trade quantity must be positive")
	}
	if trade.BuyerParticipantID == "" || trade.SellerParticipantID == "" {
		return NovationResult{}, fmt.Errorf("novation: both buyer and seller participant IDs are required")
	}

	now := time.Now()
	value := trade.Price.MulUint64(trade.Quantity)

	buyerObl := types.ClearingObligation{
		ObligationID:  s.idGen.NewID(),
		TradeID:       trade.TradeID,
		InstrumentID:  trade.InstrumentID,
		ParticipantID: trade.BuyerParticipantID,
		Side:          types.SideBuy,
		Price:         trade.Price,
		Quantity:      trade.Quantity,
		Value:         value,
		Status:        types.ClearingStatusNovated,
		CreatedAt:     now,
		NovatedAt:     now,
	}

	sellerObl := types.ClearingObligation{
		ObligationID:  s.idGen.NewID(),
		TradeID:       trade.TradeID,
		InstrumentID:  trade.InstrumentID,
		ParticipantID: trade.SellerParticipantID,
		Side:          types.SideSell,
		Price:         trade.Price,
		Quantity:      trade.Quantity,
		Value:         value,
		Status:        types.ClearingStatusNovated,
		CreatedAt:     now,
		NovatedAt:     now,
	}

	return NovationResult{
		BuyerObligation:  buyerObl,
		SellerObligation: sellerObl,
	}, nil
}
