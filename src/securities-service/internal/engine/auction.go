// Package engine — call auction matching engine for pre-open and closing auctions.
package engine

import (
	"fmt"
	"sort"
	"time"

	"github.com/garudax-platform/securities-service/internal/settlement"
	"github.com/garudax-platform/securities-service/internal/store"
	"github.com/garudax-platform/securities-service/internal/types"
)

// AuctionEngine collects orders during auction phases and executes call auctions
// to determine a single clearing price that maximises matchable volume.
type AuctionEngine struct {
	orderStore       store.OrderStore
	tradeStore       store.TradeStore
	positionStore    store.PositionStore
	settlementEngine *settlement.SettlementEngine
}

// NewAuctionEngine creates a new AuctionEngine with the given stores.
// settlementEngine may be nil; if so, settlement obligations are not created.
func NewAuctionEngine(
	orderStore store.OrderStore,
	tradeStore store.TradeStore,
	positionStore store.PositionStore,
	settlementEngine *settlement.SettlementEngine,
) *AuctionEngine {
	return &AuctionEngine{
		orderStore:       orderStore,
		tradeStore:       tradeStore,
		positionStore:    positionStore,
		settlementEngine: settlementEngine,
	}
}

// CollectOrder stores an order as PENDING without attempting to match it.
// Orders are held until RunAuction is called for the instrument.
func (a *AuctionEngine) CollectOrder(order *types.SecurityOrder) error {
	order.Status = types.OrderStatusPending
	return a.orderStore.Submit(order)
}

// RunAuction executes a call auction for the given instrument.
// It determines the clearing price that maximises matchable volume, executes
// trades at that price, and updates order statuses and positions.
func (a *AuctionEngine) RunAuction(instrumentID, tenantID string) ([]types.SecurityTrade, *types.AuctionResult, error) {
	// 1. List all PENDING orders for the instrument.
	pending, err := a.orderStore.List(store.OrderFilters{
		InstrumentID: instrumentID,
		Status:       types.OrderStatusPending,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list pending orders: %w", err)
	}

	// 2. Separate into buys and sells.
	var buys, sells []types.SecurityOrder
	for _, o := range pending {
		switch o.Side {
		case types.OrderSideBuy:
			buys = append(buys, o)
		case types.OrderSideSell:
			sells = append(sells, o)
		}
	}

	// If either side is empty, no auction can execute.
	if len(buys) == 0 || len(sells) == 0 {
		totalBuyVol := 0
		for _, b := range buys {
			totalBuyVol += b.Quantity - b.FilledQuantity
		}
		totalSellVol := 0
		for _, s := range sells {
			totalSellVol += s.Quantity - s.FilledQuantity
		}
		result := &types.AuctionResult{
			InstrumentID:        instrumentID,
			ClearingPrice:       types.Decimal{},
			MatchedVolume:       0,
			UnmatchedBuyVolume:  totalBuyVol,
			UnmatchedSellVolume: totalSellVol,
			TradeCount:          0,
		}
		return nil, result, nil
	}

	// 3. Sort buys descending by price, sells ascending by price.
	sort.Slice(buys, func(i, j int) bool {
		if !buys[i].Price.Equal(buys[j].Price) {
			return buys[i].Price.GreaterThan(buys[j].Price)
		}
		return buys[i].CreatedAt < buys[j].CreatedAt
	})
	sort.Slice(sells, func(i, j int) bool {
		if !sells[i].Price.Equal(sells[j].Price) {
			return sells[i].Price.LessThan(sells[j].Price)
		}
		return sells[i].CreatedAt < sells[j].CreatedAt
	})

	// 4. Build price ladder — collect all unique prices. Decimal is a comparable
	// struct so it can key a map and be sorted by its exact fixed-point value.
	priceSet := make(map[types.Decimal]bool)
	for _, b := range buys {
		priceSet[b.Price] = true
	}
	for _, s := range sells {
		priceSet[s.Price] = true
	}
	prices := make([]types.Decimal, 0, len(priceSet))
	for p := range priceSet {
		prices = append(prices, p)
	}
	sort.Slice(prices, func(i, j int) bool { return prices[i].LessThan(prices[j]) })

	// 5. For each candidate price, compute cumulative buy/sell quantities and matchable volume.
	var bestPrice types.Decimal
	bestVolume := 0

	for _, candidate := range prices {
		// cumBuyQty = sum of remaining qty for buy orders with price >= candidate
		cumBuyQty := 0
		for _, b := range buys {
			if b.Price.GreaterThanOrEqual(candidate) {
				cumBuyQty += b.Quantity - b.FilledQuantity
			}
		}
		// cumSellQty = sum of remaining qty for sell orders with price <= candidate
		cumSellQty := 0
		for _, s := range sells {
			if s.Price.LessThanOrEqual(candidate) {
				cumSellQty += s.Quantity - s.FilledQuantity
			}
		}

		matchable := cumBuyQty
		if cumSellQty < matchable {
			matchable = cumSellQty
		}

		// 6. ClearingPrice = price with max matchable volume (tie: highest price).
		if matchable > bestVolume || (matchable == bestVolume && candidate.GreaterThan(bestPrice)) {
			bestVolume = matchable
			bestPrice = candidate
		}
	}

	// No matchable volume — return empty result.
	if bestVolume == 0 {
		totalBuyVol := 0
		for _, b := range buys {
			totalBuyVol += b.Quantity - b.FilledQuantity
		}
		totalSellVol := 0
		for _, s := range sells {
			totalSellVol += s.Quantity - s.FilledQuantity
		}
		result := &types.AuctionResult{
			InstrumentID:        instrumentID,
			ClearingPrice:       types.Decimal{},
			MatchedVolume:       0,
			UnmatchedBuyVolume:  totalBuyVol,
			UnmatchedSellVolume: totalSellVol,
			TradeCount:          0,
		}
		return nil, result, nil
	}

	// 7. Execute matches at ClearingPrice.
	// Filter eligible orders: buys with price >= clearingPrice, sells with price <= clearingPrice.
	var eligibleBuys []*types.SecurityOrder
	for i := range buys {
		if buys[i].Price.GreaterThanOrEqual(bestPrice) {
			eligibleBuys = append(eligibleBuys, &buys[i])
		}
	}
	var eligibleSells []*types.SecurityOrder
	for i := range sells {
		if sells[i].Price.LessThanOrEqual(bestPrice) {
			eligibleSells = append(eligibleSells, &sells[i])
		}
	}

	var trades []types.SecurityTrade
	now := time.Now().UTC()
	sellIdx := 0
	matchedVolume := 0

	for _, buy := range eligibleBuys {
		buyRemaining := buy.Quantity - buy.FilledQuantity
		if buyRemaining <= 0 {
			continue
		}

		for sellIdx < len(eligibleSells) && buyRemaining > 0 {
			sell := eligibleSells[sellIdx]
			sellRemaining := sell.Quantity - sell.FilledQuantity
			if sellRemaining <= 0 {
				sellIdx++
				continue
			}

			fillQty := buyRemaining
			if sellRemaining < fillQty {
				fillQty = sellRemaining
			}

			// Don't exceed the total matchable volume.
			if matchedVolume+fillQty > bestVolume {
				fillQty = bestVolume - matchedVolume
			}
			if fillQty <= 0 {
				break
			}

			tradeID, err := newUUID()
			if err != nil {
				return trades, nil, fmt.Errorf("failed to generate trade ID: %w", err)
			}

			trade := types.SecurityTrade{
				ID:             tradeID,
				BuyOrderID:     buy.ID,
				SellOrderID:    sell.ID,
				InstrumentID:   instrumentID,
				Price:          bestPrice,
				Quantity:       fillQty,
				TradeDate:      now.Format("2006-01-02"),
				SettlementDate: now.AddDate(0, 0, 2).Format("2006-01-02"),
				Status:         types.TradeStatusPending,
				CreatedAt:      now.Format(time.RFC3339),
			}

			if err := a.tradeStore.Create(&trade); err != nil {
				return trades, nil, fmt.Errorf("failed to store trade: %w", err)
			}
			trades = append(trades, trade)

			// Update fill quantities.
			buy.FilledQuantity += fillQty
			sell.FilledQuantity += fillQty
			buyRemaining -= fillQty
			matchedVolume += fillQty

			if sell.FilledQuantity >= sell.Quantity {
				sellIdx++
			}
		}

		if matchedVolume >= bestVolume {
			break
		}
	}

	// 8. Update order statuses and create positions.
	for _, buy := range eligibleBuys {
		if buy.FilledQuantity > 0 {
			if buy.FilledQuantity >= buy.Quantity {
				buy.Status = types.OrderStatusFilled
			} else {
				buy.Status = types.OrderStatusPartiallyFilled
			}
			buy.UpdatedAt = now.Format(time.RFC3339)
			if err := a.orderStore.Update(buy); err != nil {
				return trades, nil, fmt.Errorf("failed to update buy order: %w", err)
			}
		}
	}
	for _, sell := range eligibleSells {
		if sell.FilledQuantity > 0 {
			if sell.FilledQuantity >= sell.Quantity {
				sell.Status = types.OrderStatusFilled
			} else {
				sell.Status = types.OrderStatusPartiallyFilled
			}
			sell.UpdatedAt = now.Format(time.RFC3339)
			if err := a.orderStore.Update(sell); err != nil {
				return trades, nil, fmt.Errorf("failed to update sell order: %w", err)
			}
		}
	}

	// Update positions for each trade.
	for _, trade := range trades {
		if err := a.updatePositions(trade); err != nil {
			return trades, nil, fmt.Errorf("failed to update positions: %w", err)
		}
	}

	// 9. Create settlement obligations.
	if a.settlementEngine != nil && len(trades) > 0 {
		if err := a.settlementEngine.CreateObligationsFromTrades(trades); err != nil {
			// Non-fatal: log but don't fail the auction.
			_ = err
		}
	}

	// Compute unmatched volumes.
	totalBuyVol := 0
	for _, b := range buys {
		totalBuyVol += b.Quantity - b.FilledQuantity
	}
	totalSellVol := 0
	for _, s := range sells {
		totalSellVol += s.Quantity - s.FilledQuantity
	}

	result := &types.AuctionResult{
		InstrumentID:        instrumentID,
		ClearingPrice:       bestPrice,
		MatchedVolume:       matchedVolume,
		UnmatchedBuyVolume:  totalBuyVol,
		UnmatchedSellVolume: totalSellVol,
		TradeCount:          len(trades),
	}

	return trades, result, nil
}

// updatePositions adjusts buyer and seller positions after an auction trade.
func (a *AuctionEngine) updatePositions(trade types.SecurityTrade) error {
	// Look up the orders to get participant IDs.
	buyOrder, err := a.orderStore.Get(trade.BuyOrderID)
	if err != nil {
		return fmt.Errorf("failed to get buy order: %w", err)
	}
	sellOrder, err := a.orderStore.Get(trade.SellOrderID)
	if err != nil {
		return fmt.Errorf("failed to get sell order: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)

	// Update buyer position: +quantity.
	buyPos, err := a.positionStore.GetOrCreate(buyOrder.ParticipantID, trade.InstrumentID)
	if err != nil {
		return fmt.Errorf("failed to get buyer position: %w", err)
	}
	oldQty := buyPos.Quantity
	oldAvgCost := buyPos.AvgCost
	buyPos.Quantity += trade.Quantity
	if buyPos.Quantity > 0 {
		// AvgCost = (oldQty*oldAvgCost + tradeQty*tradePrice) / newQty
		buyPos.AvgCost = oldAvgCost.MulInt64(int64(oldQty)).
			Add(trade.Price.MulInt64(int64(trade.Quantity))).
			DivInt64(int64(buyPos.Quantity))
	}
	buyPos.MarketValue = trade.Price.MulInt64(int64(buyPos.Quantity))
	buyPos.UnrealizedPnl = trade.Price.Sub(buyPos.AvgCost).MulInt64(int64(buyPos.Quantity))
	buyPos.UpdatedAt = now
	if err := a.positionStore.Update(buyPos); err != nil {
		return fmt.Errorf("failed to update buyer position: %w", err)
	}

	// Update seller position: -quantity.
	sellPos, err := a.positionStore.GetOrCreate(sellOrder.ParticipantID, trade.InstrumentID)
	if err != nil {
		return fmt.Errorf("failed to get seller position: %w", err)
	}
	sellPos.Quantity -= trade.Quantity
	sellPos.MarketValue = trade.Price.MulInt64(int64(sellPos.Quantity))
	if sellPos.Quantity != 0 {
		sellPos.UnrealizedPnl = trade.Price.Sub(sellPos.AvgCost).MulInt64(int64(sellPos.Quantity))
	} else {
		sellPos.UnrealizedPnl = types.Decimal{}
	}
	sellPos.UpdatedAt = now
	if err := a.positionStore.Update(sellPos); err != nil {
		return fmt.Errorf("failed to update seller position: %w", err)
	}

	return nil
}
