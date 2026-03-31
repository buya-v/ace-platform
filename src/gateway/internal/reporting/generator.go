package reporting

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
)

// PositionInput represents a single position for report generation.
type PositionInput struct {
	InstrumentID string  `json:"instrument_id"`
	Side         string  `json:"side"` // "long" or "short"
	Quantity     float64 `json:"quantity"`
	AvgPrice     float64 `json:"avg_price"`
	MarkPrice    float64 `json:"mark_price"`
}

// MarginInput represents margin data for a participant.
type MarginInput struct {
	InitialMargin     float64 `json:"initial_margin"`
	MaintenanceMargin float64 `json:"maintenance_margin"`
	MarginUsed        float64 `json:"margin_used"`
	ExcessMargin      float64 `json:"excess_margin"`
}

// FeeInput represents aggregated fee data.
type FeeInput struct {
	TradingFees  float64 `json:"trading_fees"`
	ClearingFees float64 `json:"clearing_fees"`
	DataFees     float64 `json:"data_fees"`
	OtherFees    float64 `json:"other_fees"`
}

// TradeInput represents a single trade for market summary generation.
type TradeInput struct {
	Price    float64 `json:"price"`
	Quantity float64 `json:"quantity"`
	// Timestamp is used for ordering; earlier trades come first in the slice.
}

// PositionSnapshot represents a participant's position in one instrument,
// used as input for large trader report generation.
type PositionSnapshot struct {
	ParticipantID string  `json:"participant_id"`
	InstrumentID  string  `json:"instrument_id"`
	LongQty       float64 `json:"long_qty"`
	ShortQty      float64 `json:"short_qty"`
}

// PnLDetail is a per-position P&L breakdown included in settlement statements.
type PnLDetail struct {
	InstrumentID string  `json:"instrument_id"`
	RealizedPnL  float64 `json:"realized_pnl"`
	UnrealizedPnL float64 `json:"unrealized_pnl"`
}

// GenerateSettlementStatement creates a DailyStatement from the given inputs.
// This is a pure function: it computes the statement without side effects.
func GenerateSettlementStatement(
	id string,
	participantID string,
	date string,
	positions []PositionInput,
	margin MarginInput,
	fees FeeInput,
) DailyStatement {
	// Calculate per-position P&L
	var pnlDetails []PnLDetail
	var totalUnrealized float64
	for _, pos := range positions {
		var unrealized float64
		if pos.Side == "long" {
			unrealized = (pos.MarkPrice - pos.AvgPrice) * pos.Quantity
		} else {
			unrealized = (pos.AvgPrice - pos.MarkPrice) * pos.Quantity
		}
		unrealized = roundTo4(unrealized)
		totalUnrealized += unrealized
		pnlDetails = append(pnlDetails, PnLDetail{
			InstrumentID:  pos.InstrumentID,
			RealizedPnL:   0, // realized P&L requires trade history; set to 0 for EOD snapshot
			UnrealizedPnL: unrealized,
		})
	}

	totalFees := roundTo4(fees.TradingFees + fees.ClearingFees + fees.DataFees + fees.OtherFees)
	netAmount := roundTo4(totalUnrealized - totalFees)

	positionsJSON, _ := json.Marshal(positions)
	marginJSON, _ := json.Marshal(margin)
	pnlJSON, _ := json.Marshal(map[string]interface{}{
		"details":            pnlDetails,
		"total_unrealized":   roundTo4(totalUnrealized),
		"total_realized":     0,
	})
	feesJSON, _ := json.Marshal(map[string]interface{}{
		"trading_fees":  fees.TradingFees,
		"clearing_fees": fees.ClearingFees,
		"data_fees":     fees.DataFees,
		"other_fees":    fees.OtherFees,
		"total_fees":    totalFees,
	})

	return DailyStatement{
		ID:            id,
		ParticipantID: participantID,
		ReportDate:    date,
		Positions:     positionsJSON,
		Margin:        marginJSON,
		PnL:           pnlJSON,
		Fees:          feesJSON,
		NetAmount:     netAmount,
	}
}

// GenerateMarketSummary creates a MarketSummary from a slice of trades and a settlement price.
// trades must be in chronological order (earliest first).
// This is a pure function.
func GenerateMarketSummary(
	id string,
	instrumentID string,
	date string,
	trades []TradeInput,
	settlementPrice float64,
	openInterest float64,
) MarketSummary {
	ms := MarketSummary{
		ID:              id,
		InstrumentID:    instrumentID,
		ReportDate:      date,
		SettlementPrice: roundTo4(settlementPrice),
		OpenInterest:    roundTo4(openInterest),
	}

	if len(trades) == 0 {
		return ms
	}

	ms.OpenPrice = roundTo4(trades[0].Price)
	ms.ClosePrice = roundTo4(trades[len(trades)-1].Price)

	high := trades[0].Price
	low := trades[0].Price
	var totalVolume float64

	for _, t := range trades {
		if t.Price > high {
			high = t.Price
		}
		if t.Price < low {
			low = t.Price
		}
		totalVolume += t.Quantity
	}

	ms.HighPrice = roundTo4(high)
	ms.LowPrice = roundTo4(low)
	ms.Volume = roundTo4(totalVolume)

	return ms
}

// GenerateLargeTraderReport identifies participants whose position exceeds the
// given threshold percentage of open interest in any instrument.
// Returns a slice of LargeTraderPosition entries, one per qualifying
// (participant, instrument) pair.
// This is a pure function.
func GenerateLargeTraderReport(
	date string,
	positions []PositionSnapshot,
	openInterestByInstrument map[string]float64,
	thresholdPct float64,
) []LargeTraderPosition {
	// Group positions by (participant, instrument) — they should already be
	// one-per-pair, but aggregate just in case.
	type key struct {
		participant string
		instrument  string
	}
	grouped := make(map[key]*PositionSnapshot)
	for i := range positions {
		p := &positions[i]
		k := key{p.ParticipantID, p.InstrumentID}
		if existing, ok := grouped[k]; ok {
			existing.LongQty += p.LongQty
			existing.ShortQty += p.ShortQty
		} else {
			copy := *p
			grouped[k] = &copy
		}
	}

	var result []LargeTraderPosition

	for k, snap := range grouped {
		oi := openInterestByInstrument[k.instrument]
		if oi <= 0 {
			continue
		}

		netPos := snap.LongQty - snap.ShortQty
		grossPos := snap.LongQty + snap.ShortQty
		pctOfOI := (math.Abs(netPos) / oi) * 100

		if pctOfOI >= thresholdPct {
			result = append(result, LargeTraderPosition{
				ID:                fmt.Sprintf("ltp-%s-%s-%s", k.participant, k.instrument, date),
				ParticipantID:     k.participant,
				InstrumentID:      k.instrument,
				ReportDate:        date,
				NetPosition:       roundTo4(netPos),
				GrossPosition:     roundTo4(grossPos),
				PctOfOpenInterest: roundTo4(pctOfOI),
			})
		}
	}

	// Sort for deterministic output
	sort.Slice(result, func(i, j int) bool {
		if result[i].ParticipantID != result[j].ParticipantID {
			return result[i].ParticipantID < result[j].ParticipantID
		}
		return result[i].InstrumentID < result[j].InstrumentID
	})

	return result
}

// roundTo4 rounds a float to 4 decimal places.
func roundTo4(v float64) float64 {
	return math.Round(v*10000) / 10000
}
