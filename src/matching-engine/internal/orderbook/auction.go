package orderbook

import (
	"sort"
	"time"

	"github.com/garudax-platform/matching-engine/internal/types"
)

// AuctionEngine accumulates orders during an auction phase and computes
// the equilibrium price that maximizes matched volume at uncrossing.
type AuctionEngine struct {
	idGen     IDGenerator
	globalSeq *uint64
}

// NewAuctionEngine creates a new AuctionEngine.
func NewAuctionEngine(idGen IDGenerator, globalSeq *uint64) *AuctionEngine {
	return &AuctionEngine{
		idGen:     idGen,
		globalSeq: globalSeq,
	}
}

// AuctionResult contains the output of an auction uncrossing.
type AuctionResult struct {
	EquilibriumPrice types.Decimal
	Trades           []types.Trade
	ExecutionReports []types.ExecutionReport
}

// priceVolume holds a candidate price and its maximum matchable volume.
type priceVolume struct {
	price  types.Decimal
	volume uint64
}

// RunAuction finds the equilibrium price that maximizes matched volume,
// generates trades at that price, and returns remaining orders on the book.
// The referencePrice is typically the last trade price or previous close,
// used as a tiebreaker when multiple prices yield the same maximum volume.
//
// Algorithm:
//  1. Collect all unique prices from both sides.
//  2. For each candidate price, compute cumulative bid volume (orders priced >= candidate)
//     and cumulative ask volume (orders priced <= candidate). Matchable volume = min(bidVol, askVol).
//  3. Select the price(s) with maximum matchable volume.
//  4. Tiebreaker: closest to referencePrice, then higher price wins.
//  5. Execute fills at the equilibrium price in time priority (sequence order).
func (ae *AuctionEngine) RunAuction(instrumentID string, bids, asks []*PriceLevel, referencePrice types.Decimal) AuctionResult {
	result := AuctionResult{}

	if len(bids) == 0 || len(asks) == 0 {
		return result
	}

	// Check if the book crosses at all: best bid must be >= best ask
	// bids are sorted descending (best first), asks ascending (best first)
	if bids[0].Price.LessThan(asks[0].Price) {
		return result // no crossing
	}

	// Step 1: Collect all unique candidate prices from both sides
	candidates := ae.collectCandidatePrices(bids, asks)
	if len(candidates) == 0 {
		return result
	}

	// Step 2: For each candidate price, compute matchable volume
	pvs := make([]priceVolume, 0, len(candidates))
	for _, price := range candidates {
		bidVol := cumulativeBidVolume(bids, price)
		askVol := cumulativeAskVolume(asks, price)
		matchVol := minUint64(bidVol, askVol)
		if matchVol > 0 {
			pvs = append(pvs, priceVolume{price: price, volume: matchVol})
		}
	}

	if len(pvs) == 0 {
		return result
	}

	// Step 3: Find maximum volume
	maxVol := pvs[0].volume
	for _, pv := range pvs[1:] {
		if pv.volume > maxVol {
			maxVol = pv.volume
		}
	}

	// Filter to only prices with max volume
	maxPVs := make([]priceVolume, 0)
	for _, pv := range pvs {
		if pv.volume == maxVol {
			maxPVs = append(maxPVs, pv)
		}
	}

	// Step 4: Tiebreaker — closest to reference price, then higher price
	equilibrium := ae.selectEquilibriumPrice(maxPVs, referencePrice)
	result.EquilibriumPrice = equilibrium

	// Step 5: Execute fills at equilibrium price
	result.Trades, result.ExecutionReports = ae.executeFills(instrumentID, bids, asks, equilibrium, maxVol)

	return result
}

// collectCandidatePrices returns sorted unique prices from both bid and ask levels.
func (ae *AuctionEngine) collectCandidatePrices(bids, asks []*PriceLevel) []types.Decimal {
	seen := make(map[int64]bool)
	var prices []types.Decimal

	for _, level := range bids {
		raw := level.Price.Raw()
		if !seen[raw] {
			seen[raw] = true
			prices = append(prices, level.Price)
		}
	}
	for _, level := range asks {
		raw := level.Price.Raw()
		if !seen[raw] {
			seen[raw] = true
			prices = append(prices, level.Price)
		}
	}

	// Sort ascending
	sort.Slice(prices, func(i, j int) bool {
		return prices[i].LessThan(prices[j])
	})
	return prices
}

// cumulativeBidVolume returns total bid volume for orders priced >= the candidate price.
func cumulativeBidVolume(bids []*PriceLevel, price types.Decimal) uint64 {
	var vol uint64
	for _, level := range bids {
		if level.Price.GreaterThanOrEqual(price) {
			vol += level.TotalQty
		}
	}
	return vol
}

// cumulativeAskVolume returns total ask volume for orders priced <= the candidate price.
func cumulativeAskVolume(asks []*PriceLevel, price types.Decimal) uint64 {
	var vol uint64
	for _, level := range asks {
		if level.Price.LessThanOrEqual(price) {
			vol += level.TotalQty
		}
	}
	return vol
}

// selectEquilibriumPrice picks the best price from candidates that all share
// maximum volume. Tiebreaker: closest to referencePrice, then higher price.
func (ae *AuctionEngine) selectEquilibriumPrice(candidates []priceVolume, referencePrice types.Decimal) types.Decimal {
	if len(candidates) == 1 {
		return candidates[0].price
	}

	best := candidates[0]
	bestDist := best.price.Sub(referencePrice).Abs()

	for _, pv := range candidates[1:] {
		dist := pv.price.Sub(referencePrice).Abs()
		if dist.LessThan(bestDist) {
			best = pv
			bestDist = dist
		} else if dist.Equal(bestDist) {
			// Same distance from reference: pick higher price
			if pv.price.GreaterThan(best.price) {
				best = pv
				bestDist = dist
			}
		}
	}
	return best.price
}

// executeFills generates trades at the equilibrium price by matching
// bid and ask orders in time priority (sequence number order).
// Returns trades and execution reports. Partially filled orders
// have their RemainingQty updated but stay on the book.
func (ae *AuctionEngine) executeFills(instrumentID string, bids, asks []*PriceLevel, eqPrice types.Decimal, maxVol uint64) ([]types.Trade, []types.ExecutionReport) {
	var trades []types.Trade
	var reports []types.ExecutionReport
	now := time.Now()

	// Collect eligible orders: bids priced >= eqPrice, asks priced <= eqPrice
	// Sort each side by sequence number (time priority)
	eligibleBids := collectEligibleOrders(bids, eqPrice, true)
	eligibleAsks := collectEligibleOrders(asks, eqPrice, false)

	remaining := maxVol
	bidIdx, askIdx := 0, 0

	for remaining > 0 && bidIdx < len(eligibleBids) && askIdx < len(eligibleAsks) {
		bid := eligibleBids[bidIdx]
		ask := eligibleAsks[askIdx]

		fillQty := minUint64(bid.RemainingQty, ask.RemainingQty)
		if fillQty > remaining {
			fillQty = remaining
		}

		bid.Fill(fillQty)
		ask.Fill(fillQty)
		remaining -= fillQty

		tradeID := ae.idGen.NewID()
		trade := types.Trade{
			TradeID:             tradeID,
			InstrumentID:        instrumentID,
			BuyOrderID:          bid.OrderID,
			SellOrderID:         ask.OrderID,
			BuyerParticipantID:  bid.ParticipantID,
			SellerParticipantID: ask.ParticipantID,
			Price:               eqPrice,
			Quantity:            fillQty,
			TradeValue:          eqPrice.MulUint64(fillQty),
			AggressorSide:       types.SideUnspecified, // Auction — no aggressor
			TradeType:           types.TradeTypeAuction,
			ExecutedAt:          now,
		}
		trades = append(trades, trade)

		// Execution reports for bid
		bidExecType := types.ExecTypePartialFill
		if bid.RemainingQty == 0 {
			bidExecType = types.ExecTypeFill
		}
		reports = append(reports, types.ExecutionReport{
			ExecID:        ae.idGen.NewID(),
			OrderID:       bid.OrderID,
			ClientOrderID: bid.ClientOrderID,
			ExecType:      bidExecType,
			OrderStatus:   bid.Status,
			Side:          types.SideBuy,
			InstrumentID:  instrumentID,
			Price:         bid.Price,
			Quantity:      bid.Quantity,
			LastQty:       fillQty,
			LastPrice:     eqPrice,
			CumulativeQty: bid.FilledQty,
			LeavesQty:     bid.RemainingQty,
			TradeID:       tradeID,
			TransactTime:  now,
			AccountID:     bid.AccountID,
		})

		// Execution reports for ask
		askExecType := types.ExecTypePartialFill
		if ask.RemainingQty == 0 {
			askExecType = types.ExecTypeFill
		}
		reports = append(reports, types.ExecutionReport{
			ExecID:        ae.idGen.NewID(),
			OrderID:       ask.OrderID,
			ClientOrderID: ask.ClientOrderID,
			ExecType:      askExecType,
			OrderStatus:   ask.Status,
			Side:          types.SideSell,
			InstrumentID:  instrumentID,
			Price:         ask.Price,
			Quantity:      ask.Quantity,
			LastQty:       fillQty,
			LastPrice:     eqPrice,
			CumulativeQty: ask.FilledQty,
			LeavesQty:     ask.RemainingQty,
			TradeID:       tradeID,
			TransactTime:  now,
			AccountID:     ask.AccountID,
		})

		if bid.RemainingQty == 0 {
			bidIdx++
		}
		if ask.RemainingQty == 0 {
			askIdx++
		}
	}

	return trades, reports
}

// collectEligibleOrders collects orders eligible at the equilibrium price,
// sorted by sequence number (time priority). For bids, eligible means price >= eqPrice.
// For asks, eligible means price <= eqPrice.
func collectEligibleOrders(levels []*PriceLevel, eqPrice types.Decimal, isBid bool) []*types.Order {
	var orders []*types.Order
	for _, level := range levels {
		if isBid && level.Price.GreaterThanOrEqual(eqPrice) {
			orders = append(orders, level.Orders()...)
		} else if !isBid && level.Price.LessThanOrEqual(eqPrice) {
			orders = append(orders, level.Orders()...)
		}
	}
	// Sort by sequence number (time priority)
	sort.Slice(orders, func(i, j int) bool {
		return orders[i].SequenceNumber < orders[j].SequenceNumber
	})
	return orders
}

func minUint64(a, b uint64) uint64 {
	if a < b {
		return a
	}
	return b
}
