package reporting

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"

	"github.com/garudax-platform/decimal"
)

// Money fields in this package use the platform's shared fixed-point Decimal
// type (Decimal(18,4)) rather than float64: settlement P&L, fee totals, net
// amounts and OHLC prices are money and must be aggregated without float drift,
// silent int64 overflow or truncation bias (see the R006/R022 money-path audit
// and the R020 fees/risk migration). Genuine quantities (contract counts,
// volume, open interest, net/gross positions) and ratios (percent of open
// interest) remain float64 — they are not money and do not require exact
// fixed-point arithmetic, mirroring how fees.Volume30D / risk percentages were
// left as float64.

// PositionInput represents a single position for report generation.
// AvgPrice and MarkPrice are prices (money); Quantity is a contract count.
type PositionInput struct {
	InstrumentID string          `json:"instrument_id"`
	Side         string          `json:"side"` // "long" or "short"
	Quantity     float64         `json:"quantity"`
	AvgPrice     decimal.Decimal `json:"avg_price"`
	MarkPrice    decimal.Decimal `json:"mark_price"`
}

// MarginInput represents margin data for a participant. All fields are money.
type MarginInput struct {
	InitialMargin     decimal.Decimal `json:"initial_margin"`
	MaintenanceMargin decimal.Decimal `json:"maintenance_margin"`
	MarginUsed        decimal.Decimal `json:"margin_used"`
	ExcessMargin      decimal.Decimal `json:"excess_margin"`
}

// FeeInput represents aggregated fee data. All fields are money.
type FeeInput struct {
	TradingFees  decimal.Decimal `json:"trading_fees"`
	ClearingFees decimal.Decimal `json:"clearing_fees"`
	DataFees     decimal.Decimal `json:"data_fees"`
	OtherFees    decimal.Decimal `json:"other_fees"`
}

// TradeInput represents a single trade for market summary generation.
// Price is money; Quantity is a contract count.
type TradeInput struct {
	Price    decimal.Decimal `json:"price"`
	Quantity float64         `json:"quantity"`
	// Timestamp is used for ordering; earlier trades come first in the slice.
}

// PositionSnapshot represents a participant's position in one instrument,
// used as input for large trader report generation. Quantities are contract
// counts (float64), not money.
type PositionSnapshot struct {
	ParticipantID string  `json:"participant_id"`
	InstrumentID  string  `json:"instrument_id"`
	LongQty       float64 `json:"long_qty"`
	ShortQty      float64 `json:"short_qty"`
}

// PnLDetail is a per-position P&L breakdown included in settlement statements.
// Both fields are money.
type PnLDetail struct {
	InstrumentID  string          `json:"instrument_id"`
	RealizedPnL   decimal.Decimal `json:"realized_pnl"`
	UnrealizedPnL decimal.Decimal `json:"unrealized_pnl"`
}

// GenerateSettlementStatement creates a DailyStatement from the given inputs.
// This is a pure function: it computes the statement without side effects.
//
// P&L and net amount are computed in fixed-point Decimal so settlement
// statements are exact to 4 dp. Per-position unrealized P&L is
// (mark - avg) * quantity; MulDecimal already rounds half-to-even to 4 dp, so
// no separate rounding step is needed.
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
	totalUnrealized := decimal.Zero()
	for _, pos := range positions {
		qty := decFromFloat(pos.Quantity)
		var unrealized decimal.Decimal
		if pos.Side == "long" {
			unrealized = pos.MarkPrice.Sub(pos.AvgPrice).MulDecimal(qty)
		} else {
			unrealized = pos.AvgPrice.Sub(pos.MarkPrice).MulDecimal(qty)
		}
		totalUnrealized = totalUnrealized.Add(unrealized)
		pnlDetails = append(pnlDetails, PnLDetail{
			InstrumentID:  pos.InstrumentID,
			RealizedPnL:   decimal.Zero(), // realized P&L requires trade history; 0 for EOD snapshot
			UnrealizedPnL: unrealized,
		})
	}

	totalFees := fees.TradingFees.Add(fees.ClearingFees).Add(fees.DataFees).Add(fees.OtherFees)
	netAmount := totalUnrealized.Sub(totalFees)

	positionsJSON, _ := json.Marshal(positions)
	marginJSON, _ := json.Marshal(margin)
	pnlJSON, _ := json.Marshal(map[string]interface{}{
		"details":          pnlDetails,
		"total_unrealized": totalUnrealized,
		"total_realized":   decimal.Zero(),
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
// settlementPrice is money (Decimal); openInterest is a contract count (float64).
// This is a pure function.
func GenerateMarketSummary(
	id string,
	instrumentID string,
	date string,
	trades []TradeInput,
	settlementPrice decimal.Decimal,
	openInterest float64,
) MarketSummary {
	ms := MarketSummary{
		ID:              id,
		InstrumentID:    instrumentID,
		ReportDate:      date,
		SettlementPrice: settlementPrice,
		OpenInterest:    roundTo4(openInterest),
	}

	if len(trades) == 0 {
		return ms
	}

	ms.OpenPrice = trades[0].Price
	ms.ClosePrice = trades[len(trades)-1].Price

	high := trades[0].Price
	low := trades[0].Price
	var totalVolume float64

	for _, t := range trades {
		if t.Price.GreaterThan(high) {
			high = t.Price
		}
		if t.Price.LessThan(low) {
			low = t.Price
		}
		totalVolume += t.Quantity
	}

	ms.HighPrice = high
	ms.LowPrice = low
	ms.Volume = roundTo4(totalVolume)

	return ms
}

// GenerateLargeTraderReport identifies participants whose position exceeds the
// given threshold percentage of open interest in any instrument.
// Returns a slice of LargeTraderPosition entries, one per qualifying
// (participant, instrument) pair.
//
// Positions and open interest are contract counts and the percent-of-OI is a
// ratio — none of these are money, so this report is computed in float64.
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

// roundTo4 rounds a float to 4 decimal places. It is used only for non-money
// quantity and ratio values (volume, open interest, net/gross positions,
// percent of open interest); money values use the Decimal type, which is
// already exact to 4 dp.
func roundTo4(v float64) float64 {
	return math.Round(v*10000) / 10000
}
