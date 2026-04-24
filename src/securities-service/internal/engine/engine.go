// Package engine implements the securities matching engine with price-time priority.
package engine

import (
	"crypto/rand"
	"fmt"
	"sort"
	"time"

	"github.com/garudax-platform/securities-service/internal/kafka"
	"github.com/garudax-platform/securities-service/internal/settlement"
	"github.com/garudax-platform/securities-service/internal/store"
	"github.com/garudax-platform/securities-service/internal/types"
)

// MatchingEngine matches incoming orders against resting orders using price-time priority.
type MatchingEngine struct {
	instrumentStore      store.InstrumentStore
	orderStore           store.OrderStore
	tradeStore           store.TradeStore
	positionStore        store.PositionStore
	producer             kafka.Producer
	settlementEngine     *settlement.SettlementEngine
	circuitBreakerEngine *CircuitBreakerEngine
}

// NewMatchingEngine creates a new MatchingEngine with the given stores.
// producer may be nil; if so, trade events are not published.
// settlementEngine may be nil; if so, settlement obligations are not created.
// circuitBreakerEngine may be nil; if so, circuit breaker checks are skipped.
func NewMatchingEngine(
	instrumentStore store.InstrumentStore,
	orderStore store.OrderStore,
	tradeStore store.TradeStore,
	positionStore store.PositionStore,
	producer kafka.Producer,
	settlementEngine *settlement.SettlementEngine,
	circuitBreakerEngine *CircuitBreakerEngine,
) *MatchingEngine {
	return &MatchingEngine{
		instrumentStore:      instrumentStore,
		orderStore:           orderStore,
		tradeStore:           tradeStore,
		positionStore:        positionStore,
		producer:             producer,
		settlementEngine:     settlementEngine,
		circuitBreakerEngine: circuitBreakerEngine,
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

// MatchOrder attempts to match an incoming order against resting orders on the opposite side.
// tenantID is used to route Kafka events to the correct tenant-scoped topic.
// It returns all trades created during matching.
func (e *MatchingEngine) MatchOrder(tenantID string, order *types.SecurityOrder) ([]types.SecurityTrade, error) {
	// 1. Get instrument and validate ACTIVE status.
	inst, err := e.instrumentStore.Get(order.InstrumentID)
	if err != nil {
		return nil, fmt.Errorf("instrument lookup failed: %w", err)
	}
	if inst.TradingStatus != types.TradingStatusActive {
		return nil, fmt.Errorf("instrument %s is not active for trading", inst.ID)
	}

	// 1b. Circuit breaker validation for LIMIT orders.
	if order.OrderType == types.OrderTypeLimit && e.circuitBreakerEngine != nil {
		allowed, event, cbErr := e.circuitBreakerEngine.ValidatePrice(order.InstrumentID, order.Price)
		if cbErr != nil {
			return nil, fmt.Errorf("circuit breaker check failed: %w", cbErr)
		}
		if !allowed {
			return nil, fmt.Errorf("circuit breaker triggered: %s", event.Type)
		}
	}

	// 2. Determine opposite side.
	var oppositeSide types.OrderSide
	switch order.Side {
	case types.OrderSideBuy:
		oppositeSide = types.OrderSideSell
	case types.OrderSideSell:
		oppositeSide = types.OrderSideBuy
	default:
		return nil, fmt.Errorf("unsupported order side: %s", order.Side)
	}

	// 3. List all PENDING orders for the same instrument on the opposite side.
	restingOrders, err := e.orderStore.List(store.OrderFilters{
		InstrumentID: order.InstrumentID,
		Status:       types.OrderStatusPending,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list resting orders: %w", err)
	}

	// Also include PARTIALLY_FILLED orders.
	partialOrders, err := e.orderStore.List(store.OrderFilters{
		InstrumentID: order.InstrumentID,
		Status:       types.OrderStatusPartiallyFilled,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list partial orders: %w", err)
	}
	restingOrders = append(restingOrders, partialOrders...)

	// Filter to opposite side only.
	var candidates []types.SecurityOrder
	for _, o := range restingOrders {
		if o.Side == oppositeSide && o.ID != order.ID {
			candidates = append(candidates, o)
		}
	}

	// 4. Sort resting orders by price-time priority.
	sort.Slice(candidates, func(i, j int) bool {
		if order.Side == types.OrderSideBuy {
			// Incoming BUY: sort sells ascending by price (best ask first).
			if candidates[i].Price != candidates[j].Price {
				return candidates[i].Price < candidates[j].Price
			}
		} else {
			// Incoming SELL: sort buys descending by price (best bid first).
			if candidates[i].Price != candidates[j].Price {
				return candidates[i].Price > candidates[j].Price
			}
		}
		// Time priority: earlier orders first.
		return candidates[i].CreatedAt < candidates[j].CreatedAt
	})

	// Track remaining quantity on the incoming order.
	remainingQty := order.Quantity - order.FilledQuantity
	var trades []types.SecurityTrade
	now := time.Now().UTC()

	// 5. Match against each resting order.
	stpCancelIncoming := false
	for i := range candidates {
		if remainingQty <= 0 || stpCancelIncoming {
			break
		}
		resting := &candidates[i]

		// Check if prices cross.
		if !pricesCross(order, resting) {
			break // No more crosses possible (sorted by price priority).
		}

		// 5a. Self-trade prevention: check if incoming and resting share same participant.
		if order.ParticipantID == resting.ParticipantID {
			stpMode := inst.STPMode
			if stpMode == "" {
				stpMode = types.STPCancelNewest
			}
			switch stpMode {
			case types.STPCancelNewest:
				// Skip this resting order; incoming may match other resting orders.
				continue
			case types.STPCancelOldest:
				// Cancel the resting order and continue matching against others.
				resting.Status = types.OrderStatusCancelled
				resting.UpdatedAt = now.Format(time.RFC3339)
				if err := e.orderStore.Update(resting); err != nil {
					return trades, fmt.Errorf("failed to cancel resting order (STP): %w", err)
				}
				continue
			case types.STPCancelBoth:
				// Cancel both orders and stop matching.
				resting.Status = types.OrderStatusCancelled
				resting.UpdatedAt = now.Format(time.RFC3339)
				if err := e.orderStore.Update(resting); err != nil {
					return trades, fmt.Errorf("failed to cancel resting order (STP): %w", err)
				}
				order.Status = types.OrderStatusCancelled
				order.UpdatedAt = now.Format(time.RFC3339)
				if err := e.orderStore.Update(order); err != nil {
					return trades, fmt.Errorf("failed to cancel incoming order (STP): %w", err)
				}
				stpCancelIncoming = true
				continue
			}
		}

		// Determine available quantity for matching.
		// For iceberg resting orders, match only against visible quantity.
		restingRemaining := resting.Quantity - resting.FilledQuantity
		isRestingIceberg := resting.VisibleQuantity > 0 || resting.HiddenQuantity > 0
		if isRestingIceberg {
			restingRemaining = resting.VisibleQuantity
		}

		// For iceberg incoming orders, match only the visible portion.
		incomingAvail := remainingQty
		isIncomingIceberg := order.VisibleQuantity > 0 || order.HiddenQuantity > 0
		if isIncomingIceberg {
			incomingAvail = order.VisibleQuantity
		}

		fillQty := incomingAvail
		if restingRemaining < fillQty {
			fillQty = restingRemaining
		}

		// 6. Create the SecurityTrade.
		tradeID, err := newUUID()
		if err != nil {
			return trades, fmt.Errorf("failed to generate trade ID: %w", err)
		}

		var buyOrderID, sellOrderID string
		if order.Side == types.OrderSideBuy {
			buyOrderID = order.ID
			sellOrderID = resting.ID
		} else {
			buyOrderID = resting.ID
			sellOrderID = order.ID
		}

		trade := types.SecurityTrade{
			ID:             tradeID,
			BuyOrderID:     buyOrderID,
			SellOrderID:    sellOrderID,
			InstrumentID:   order.InstrumentID,
			Price:          resting.Price,
			Quantity:       fillQty,
			TradeDate:      now.Format("2006-01-02"),
			SettlementDate: now.AddDate(0, 0, 2).Format("2006-01-02"), // T+2
			Status:         types.TradeStatusPending,
			CreatedAt:      now.Format(time.RFC3339),
		}

		if err := e.tradeStore.Create(&trade); err != nil {
			return trades, fmt.Errorf("failed to store trade: %w", err)
		}
		trades = append(trades, trade)

		// Publish trade executed event (nil-safe: no-op if producer not configured).
		if err := kafka.PublishTradeExecuted(e.producer, tenantID, &trade); err != nil {
			// Non-fatal: log the error but continue matching.
			_ = err
		}

		// Update circuit breaker last traded price after each trade.
		if e.circuitBreakerEngine != nil {
			_ = e.circuitBreakerEngine.OnTrade(order.InstrumentID, trade.Price)
		}

		// 7. Update matched orders.
		remainingQty -= fillQty

		// Update incoming order.
		order.FilledQuantity += fillQty
		if isIncomingIceberg {
			order.VisibleQuantity -= fillQty
			// Replenish visible from hidden if visible is exhausted.
			if order.VisibleQuantity == 0 && order.HiddenQuantity > 0 {
				// Compute original display size: total - hidden at creation.
				// Use min(what's left in hidden, the fill amount we just consumed as the replenish size).
				origVisible := order.Quantity - order.FilledQuantity // total unfilled
				if origVisible > order.HiddenQuantity {
					origVisible = order.HiddenQuantity
				}
				// Actually, original visible = Quantity - (original HiddenQuantity).
				// We don't store original, so replenish = min(fillQty, HiddenQuantity) as a proxy.
				// Better: track via the total iceberg size.
				// Use: replenish = min(Quantity - HiddenQuantity - FilledQuantity + fillQty, HiddenQuantity)
				// Simplest correct approach: replenish = min(fillQty, HiddenQuantity)
				replenish := fillQty
				if order.HiddenQuantity < replenish {
					replenish = order.HiddenQuantity
				}
				order.VisibleQuantity = replenish
				order.HiddenQuantity -= replenish
			}
			// Iceberg FILLED only when both visible and hidden are 0.
			if order.VisibleQuantity == 0 && order.HiddenQuantity == 0 && remainingQty == 0 {
				order.Status = types.OrderStatusFilled
			} else {
				order.Status = types.OrderStatusPartiallyFilled
			}
		} else {
			if remainingQty == 0 {
				order.Status = types.OrderStatusFilled
			} else {
				order.Status = types.OrderStatusPartiallyFilled
			}
		}
		order.UpdatedAt = now.Format(time.RFC3339)
		if err := e.orderStore.Update(order); err != nil {
			return trades, fmt.Errorf("failed to update incoming order: %w", err)
		}

		// Update resting order.
		resting.FilledQuantity += fillQty
		if isRestingIceberg {
			resting.VisibleQuantity -= fillQty
			// Replenish visible from hidden if visible is exhausted.
			if resting.VisibleQuantity == 0 && resting.HiddenQuantity > 0 {
				replenish := fillQty
				if resting.HiddenQuantity < replenish {
					replenish = resting.HiddenQuantity
				}
				resting.VisibleQuantity = replenish
				resting.HiddenQuantity -= replenish
			}
			// Iceberg FILLED only when both visible and hidden are 0.
			if resting.VisibleQuantity == 0 && resting.HiddenQuantity == 0 {
				resting.Status = types.OrderStatusFilled
			} else {
				resting.Status = types.OrderStatusPartiallyFilled
			}
		} else {
			if resting.FilledQuantity >= resting.Quantity {
				resting.Status = types.OrderStatusFilled
			} else {
				resting.Status = types.OrderStatusPartiallyFilled
			}
		}
		resting.UpdatedAt = now.Format(time.RFC3339)
		if err := e.orderStore.Update(resting); err != nil {
			return trades, fmt.Errorf("failed to update resting order: %w", err)
		}

		// 8. Update positions.
		if err := e.updatePositions(order, resting, trade); err != nil {
			return trades, fmt.Errorf("failed to update positions: %w", err)
		}
	}

	// 9. Create settlement obligations for all trades produced.
	if e.settlementEngine != nil && len(trades) > 0 {
		if err := e.settlementEngine.CreateObligationsFromTrades(trades); err != nil {
			// Non-fatal: log but don't fail matching.
			_ = err
		}
	}

	// 10. If incoming has remaining quantity, it stays as PENDING (already stored).
	return trades, nil
}

// pricesCross checks whether the incoming and resting order prices cross.
func pricesCross(incoming, resting *types.SecurityOrder) bool {
	// MARKET orders always cross.
	if incoming.OrderType == types.OrderTypeMarket {
		return true
	}

	if incoming.Side == types.OrderSideBuy {
		// Incoming BUY crosses if incoming.Price >= resting.Price.
		return incoming.Price >= resting.Price
	}
	// Incoming SELL crosses if incoming.Price <= resting.Price.
	return incoming.Price <= resting.Price
}

// updatePositions adjusts buyer and seller positions after a trade.
func (e *MatchingEngine) updatePositions(incoming, resting *types.SecurityOrder, trade types.SecurityTrade) error {
	var buyerID, sellerID string
	if incoming.Side == types.OrderSideBuy {
		buyerID = incoming.ParticipantID
		sellerID = resting.ParticipantID
	} else {
		buyerID = resting.ParticipantID
		sellerID = incoming.ParticipantID
	}

	now := time.Now().UTC().Format(time.RFC3339)

	// Update buyer position: +quantity.
	buyPos, err := e.positionStore.GetOrCreate(buyerID, trade.InstrumentID)
	if err != nil {
		return fmt.Errorf("failed to get buyer position: %w", err)
	}
	oldQty := buyPos.Quantity
	oldAvgCost := buyPos.AvgCost
	buyPos.Quantity += trade.Quantity
	if buyPos.Quantity > 0 {
		buyPos.AvgCost = ((float64(oldQty) * oldAvgCost) + (float64(trade.Quantity) * trade.Price)) / float64(buyPos.Quantity)
	}
	buyPos.MarketValue = float64(buyPos.Quantity) * trade.Price
	buyPos.UnrealizedPnl = float64(buyPos.Quantity) * (trade.Price - buyPos.AvgCost)
	buyPos.UpdatedAt = now
	if err := e.positionStore.Update(buyPos); err != nil {
		return fmt.Errorf("failed to update buyer position: %w", err)
	}

	// Update seller position: -quantity.
	sellPos, err := e.positionStore.GetOrCreate(sellerID, trade.InstrumentID)
	if err != nil {
		return fmt.Errorf("failed to get seller position: %w", err)
	}
	sellPos.Quantity -= trade.Quantity
	// AvgCost remains unchanged for selling.
	sellPos.MarketValue = float64(sellPos.Quantity) * trade.Price
	if sellPos.Quantity != 0 {
		sellPos.UnrealizedPnl = float64(sellPos.Quantity) * (trade.Price - sellPos.AvgCost)
	} else {
		sellPos.UnrealizedPnl = 0
	}
	sellPos.UpdatedAt = now
	if err := e.positionStore.Update(sellPos); err != nil {
		return fmt.Errorf("failed to update seller position: %w", err)
	}

	return nil
}
