package netting

import (
	"time"

	"github.com/garudax-platform/clearing-engine/internal/types"
)

// Service performs multilateral netting of clearing obligations.
// Netting reduces the gross number of settlement obligations to net amounts,
// decreasing settlement risk and capital requirements.
type Service struct{}

func NewService() *Service {
	return &Service{}
}

// Net takes a set of clearing obligations and produces net results per
// participant per instrument. This is multilateral netting — all obligations
// for the same participant/instrument are collapsed into a single net obligation.
func (s *Service) Net(obligations []types.ClearingObligation) []types.NettingResult {
	type key struct {
		ParticipantID string
		InstrumentID  string
	}

	type accumulator struct {
		netQty    int64
		netValue  int64 // raw decimal
		longQty   uint64
		shortQty  uint64
		count     int
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
		if obl.Side == types.SideBuy {
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
	results := make([]types.NettingResult, 0, len(accum))
	for k, a := range accum {
		results = append(results, types.NettingResult{
			ParticipantID:    k.ParticipantID,
			InstrumentID:     k.InstrumentID,
			NetQuantity:      a.netQty,
			NetValue:         types.DecimalFromRaw(a.netValue),
			GrossLongQty:     a.longQty,
			GrossShortQty:    a.shortQty,
			ObligationsCount: a.count,
			NettedAt:         now,
		})
	}

	return results
}
