package engine

import (
	"fmt"
	"time"
)

// ---------------------------------------------------------------------------
// Novation — CCP trade novation
// ---------------------------------------------------------------------------

const CCP = "GarudaX-CCP"

type NovationResult struct {
	BuyerObligation  ClearingObligation
	SellerObligation ClearingObligation
}

type NovationService struct {
	idGen IDGenerator
}

func NewNovationService(idGen IDGenerator) *NovationService {
	return &NovationService{idGen: idGen}
}

func (s *NovationService) Novate(trade Trade) (NovationResult, error) {
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

	return NovationResult{
		BuyerObligation: ClearingObligation{
			ObligationID: s.idGen.NewID(), TradeID: trade.TradeID,
			InstrumentID: trade.InstrumentID, ParticipantID: trade.BuyerParticipantID,
			Side: SideBuy, Price: trade.Price, Quantity: trade.Quantity,
			Value: value, Status: ClearingStatusNovated, CreatedAt: now, NovatedAt: now,
		},
		SellerObligation: ClearingObligation{
			ObligationID: s.idGen.NewID(), TradeID: trade.TradeID,
			InstrumentID: trade.InstrumentID, ParticipantID: trade.SellerParticipantID,
			Side: SideSell, Price: trade.Price, Quantity: trade.Quantity,
			Value: value, Status: ClearingStatusNovated, CreatedAt: now, NovatedAt: now,
		},
	}, nil
}

// ---------------------------------------------------------------------------
// Netting — multilateral obligation netting
// ---------------------------------------------------------------------------

type NettingService struct{}

func NewNettingService() *NettingService { return &NettingService{} }

func (s *NettingService) Net(obligations []ClearingObligation) []NettingResult {
	type key struct {
		ParticipantID string
		InstrumentID  string
	}
	type accumulator struct {
		netQty   int64
		netValue int64
		longQty  uint64
		shortQty uint64
		count    int
	}

	accum := make(map[key]*accumulator)
	for _, obl := range obligations {
		k := key{ParticipantID: obl.ParticipantID, InstrumentID: obl.InstrumentID}
		a, exists := accum[k]
		if !exists {
			a = &accumulator{}
			accum[k] = a
		}
		a.count++
		if obl.Side == SideBuy {
			a.netQty += int64(obl.Quantity)
			a.netValue += obl.Value.Raw()
			a.longQty += obl.Quantity
		} else {
			a.netQty -= int64(obl.Quantity)
			a.netValue -= obl.Value.Raw()
			a.shortQty += obl.Quantity
		}
	}

	now := time.Now()
	results := make([]NettingResult, 0, len(accum))
	for k, a := range accum {
		results = append(results, NettingResult{
			ParticipantID: k.ParticipantID, InstrumentID: k.InstrumentID,
			NetQuantity: a.netQty, NetValue: DecimalFromRaw(a.netValue),
			GrossLongQty: a.longQty, GrossShortQty: a.shortQty,
			ObligationsCount: a.count, NettedAt: now,
		})
	}
	return results
}
