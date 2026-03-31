package orderbook

import (
	"fmt"
	"sort"
	"sync/atomic"
	"time"

	"github.com/garudax-platform/matching-engine/internal/types"
)

// IDGenerator generates unique IDs. In production, use UUID v7.
// This interface allows injection for testing.
type IDGenerator interface {
	NewID() string
}

// MatchResult contains the output of processing an incoming order.
type MatchResult struct {
	Trades           []types.Trade
	ExecutionReports []types.ExecutionReport
}

// OrderBook implements a Central Limit Order Book with price-time priority.
// Single-threaded per instrument — callers must synchronize.
type OrderBook struct {
	InstrumentID   string
	State          types.BookState
	LastTradePrice types.Decimal

	bids []*PriceLevel // Sorted descending by price (best bid first)
	asks []*PriceLevel // Sorted ascending by price (best ask first)

	// orderIndex maps order ID to the order for O(1) lookup/cancel
	orderIndex map[string]*types.Order
	// orderLevelIndex maps order ID to its price level for fast removal
	orderLevelIndex map[string]*PriceLevel

	sequenceCounter uint64 // per-book trade sequence
	idGen           IDGenerator

	// Global sequence counter shared across all books (pointer to engine-level counter)
	globalSeq *uint64
}

// NewOrderBook creates a new order book for an instrument.
func NewOrderBook(instrumentID string, idGen IDGenerator, globalSeq *uint64) *OrderBook {
	return &OrderBook{
		InstrumentID:    instrumentID,
		State:           types.BookStateContinuous,
		bids:            make([]*PriceLevel, 0),
		asks:            make([]*PriceLevel, 0),
		orderIndex:      make(map[string]*types.Order),
		orderLevelIndex: make(map[string]*PriceLevel),
		idGen:           idGen,
		globalSeq:       globalSeq,
	}
}

// nextSequence atomically increments and returns the next global sequence number.
func (ob *OrderBook) nextSequence() uint64 {
	return atomic.AddUint64(ob.globalSeq, 1)
}

// nextTradeSequence increments and returns the next per-book trade sequence.
func (ob *OrderBook) nextTradeSequence() uint64 {
	ob.sequenceCounter++
	return ob.sequenceCounter
}

// SubmitOrder processes an incoming order through validation and matching.
func (ob *OrderBook) SubmitOrder(order *types.Order) MatchResult {
	result := MatchResult{}
	now := time.Now()

	// Assign sequence number
	order.SequenceNumber = ob.nextSequence()
	order.CreatedAt = now

	// Validate order
	if reason := ob.validateOrder(order); reason != "" {
		order.Status = types.OrderStatusRejected
		result.ExecutionReports = append(result.ExecutionReports, types.ExecutionReport{
			ExecID:        ob.idGen.NewID(),
			OrderID:       order.OrderID,
			ClientOrderID: order.ClientOrderID,
			ExecType:      types.ExecTypeRejected,
			OrderStatus:   types.OrderStatusRejected,
			Side:          order.Side,
			InstrumentID:  ob.InstrumentID,
			Price:         order.Price,
			Quantity:      order.Quantity,
			LeavesQty:     order.Quantity,
			TransactTime:  now,
			RejectReason:  reason,
			AccountID:     order.AccountID,
		})
		return result
	}

	// Accept order
	order.Status = types.OrderStatusNew
	order.RemainingQty = order.Quantity

	// Initialize iceberg fields (adjusts RemainingQty to DisplayQty)
	initIceberg(order)

	// Emit NEW execution report
	result.ExecutionReports = append(result.ExecutionReports, types.ExecutionReport{
		ExecID:        ob.idGen.NewID(),
		OrderID:       order.OrderID,
		ClientOrderID: order.ClientOrderID,
		ExecType:      types.ExecTypeNew,
		OrderStatus:   types.OrderStatusNew,
		Side:          order.Side,
		InstrumentID:  ob.InstrumentID,
		Price:         order.Price,
		Quantity:      order.Quantity,
		LeavesQty:     order.RemainingQty,
		TransactTime:  now,
		AccountID:     order.AccountID,
	})

	// During auction/pre-open phases, queue orders on the book without matching.
	// Only LIMIT orders are accepted during non-continuous phases.
	if ob.State == types.BookStateAuction || ob.State == types.BookStatePreOpen {
		if order.OrderType != types.OrderTypeLimit {
			order.Status = types.OrderStatusRejected
			result.ExecutionReports = append(result.ExecutionReports, types.ExecutionReport{
				ExecID:        ob.idGen.NewID(),
				OrderID:       order.OrderID,
				ClientOrderID: order.ClientOrderID,
				ExecType:      types.ExecTypeRejected,
				OrderStatus:   types.OrderStatusRejected,
				Side:          order.Side,
				InstrumentID:  ob.InstrumentID,
				Price:         order.Price,
				Quantity:      order.Quantity,
				LeavesQty:     order.Quantity,
				TransactTime:  now,
				RejectReason:  "only limit orders accepted during auction/pre-open phase",
				AccountID:     order.AccountID,
			})
			return result
		}
		ob.addToBook(order)
		return result
	}

	// FOK: check full fillability before matching
	if order.TimeInForce == types.TIFFOK {
		if !ob.canFillFOK(order) {
			order.Status = types.OrderStatusCancelled
			result.ExecutionReports = append(result.ExecutionReports, types.ExecutionReport{
				ExecID:        ob.idGen.NewID(),
				OrderID:       order.OrderID,
				ClientOrderID: order.ClientOrderID,
				ExecType:      types.ExecTypeCancelled,
				OrderStatus:   types.OrderStatusCancelled,
				Side:          order.Side,
				InstrumentID:  ob.InstrumentID,
				Price:         order.Price,
				Quantity:      order.Quantity,
				LeavesQty:     order.RemainingQty,
				TransactTime:  now,
				RejectReason:  "FOK order cannot be fully filled",
				AccountID:     order.AccountID,
			})
			return result
		}
	}

	// Match against opposite side
	ob.match(order, &result, now)

	// Handle remaining quantity
	if order.RemainingQty > 0 {
		if order.OrderType == types.OrderTypeMarket || order.TimeInForce == types.TIFIOC {
			// MARKET and IOC orders don't rest on the book
			order.Status = types.OrderStatusCancelled
			result.ExecutionReports = append(result.ExecutionReports, types.ExecutionReport{
				ExecID:        ob.idGen.NewID(),
				OrderID:       order.OrderID,
				ClientOrderID: order.ClientOrderID,
				ExecType:      types.ExecTypeCancelled,
				OrderStatus:   types.OrderStatusCancelled,
				Side:          order.Side,
				InstrumentID:  ob.InstrumentID,
				Price:         order.Price,
				Quantity:      order.Quantity,
				LastQty:       0,
				CumulativeQty: order.FilledQty,
				LeavesQty:     0,
				TransactTime:  now,
				AccountID:     order.AccountID,
			})
		} else {
			// LIMIT order rests on the book
			ob.addToBook(order)
		}
	}

	return result
}

// CancelOrder cancels an existing order by ID.
func (ob *OrderBook) CancelOrder(orderID string) (types.ExecutionReport, error) {
	order, ok := ob.orderIndex[orderID]
	if !ok {
		return types.ExecutionReport{}, fmt.Errorf("order %s not found", orderID)
	}

	// Remove from price level
	level, ok := ob.orderLevelIndex[orderID]
	if ok {
		level.RemoveOrder(orderID)
		if level.IsEmpty() {
			ob.removePriceLevel(order.Side, level.Price)
		}
	}

	delete(ob.orderIndex, orderID)
	delete(ob.orderLevelIndex, orderID)

	order.Status = types.OrderStatusCancelled
	now := time.Now()

	return types.ExecutionReport{
		ExecID:        ob.idGen.NewID(),
		OrderID:       order.OrderID,
		ClientOrderID: order.ClientOrderID,
		ExecType:      types.ExecTypeCancelled,
		OrderStatus:   types.OrderStatusCancelled,
		Side:          order.Side,
		InstrumentID:  ob.InstrumentID,
		Price:         order.Price,
		Quantity:      order.Quantity,
		CumulativeQty: order.FilledQty,
		LeavesQty:     0,
		TransactTime:  now,
		AccountID:     order.AccountID,
	}, nil
}

// CancelAll cancels all orders matching the given filters.
// Returns the number of cancelled orders and their IDs.
func (ob *OrderBook) CancelAll(accountID string, side types.Side) (uint32, []string) {
	var cancelled uint32
	var cancelledIDs []string

	// Collect matching order IDs first to avoid mutating map during iteration
	var toCancel []string
	for id, order := range ob.orderIndex {
		if order.AccountID != accountID {
			continue
		}
		if side != types.SideUnspecified && order.Side != side {
			continue
		}
		toCancel = append(toCancel, id)
	}

	for _, id := range toCancel {
		if _, err := ob.CancelOrder(id); err == nil {
			cancelled++
			cancelledIDs = append(cancelledIDs, id)
		}
	}

	return cancelled, cancelledIDs
}

// ModifyOrder implements cancel-replace semantics. The existing order is cancelled
// and a new order is submitted with the updated price/quantity (loses time priority).
func (ob *OrderBook) ModifyOrder(orderID, accountID string, newPrice types.Decimal, newQty uint64) (MatchResult, error) {
	order, ok := ob.orderIndex[orderID]
	if !ok {
		return MatchResult{}, fmt.Errorf("order %s not found", orderID)
	}
	if order.AccountID != accountID {
		return MatchResult{}, fmt.Errorf("order %s not owned by account %s", orderID, accountID)
	}

	// Capture order details before cancelling
	newOrder := &types.Order{
		OrderID:       ob.idGen.NewID(),
		ClientOrderID: order.ClientOrderID,
		InstrumentID:  order.InstrumentID,
		AccountID:     order.AccountID,
		ParticipantID: order.ParticipantID,
		Side:          order.Side,
		OrderType:     order.OrderType,
		TimeInForce:   order.TimeInForce,
		Price:         order.Price,
		StopPrice:     order.StopPrice,
		Quantity:      order.RemainingQty, // Use remaining as new quantity
		STPMode:       order.STPMode,
		ExpireAt:      order.ExpireAt,
	}

	if !newPrice.IsZero() {
		newOrder.Price = newPrice
	}
	if newQty > 0 {
		newOrder.Quantity = newQty
	}

	// Cancel original
	cancelReport, err := ob.CancelOrder(orderID)
	if err != nil {
		return MatchResult{}, err
	}

	// Submit replacement
	result := ob.SubmitOrder(newOrder)
	// Prepend the cancel report
	result.ExecutionReports = append([]types.ExecutionReport{cancelReport}, result.ExecutionReports...)

	return result, nil
}

// GetOrder returns an order by ID.
func (ob *OrderBook) GetOrder(orderID string) (*types.Order, bool) {
	o, ok := ob.orderIndex[orderID]
	return o, ok
}

// BestBid returns the best bid price, or zero if no bids.
func (ob *OrderBook) BestBid() types.Decimal {
	if len(ob.bids) == 0 {
		return types.DecimalZero()
	}
	return ob.bids[0].Price
}

// BestAsk returns the best ask price, or zero if no asks.
func (ob *OrderBook) BestAsk() types.Decimal {
	if len(ob.asks) == 0 {
		return types.DecimalZero()
	}
	return ob.asks[0].Price
}

// BidLevels returns the bid price levels (best first).
func (ob *OrderBook) BidLevels() []*PriceLevel {
	return ob.bids
}

// AskLevels returns the ask price levels (best first).
func (ob *OrderBook) AskLevels() []*PriceLevel {
	return ob.asks
}

// OrderCount returns the total number of resting orders.
func (ob *OrderBook) OrderCount() int {
	return len(ob.orderIndex)
}

// validateOrder performs pre-trade validation.
func (ob *OrderBook) validateOrder(order *types.Order) string {
	if ob.State != types.BookStateContinuous && ob.State != types.BookStateAuction && ob.State != types.BookStatePreOpen {
		return fmt.Sprintf("book state %d does not accept orders", ob.State)
	}
	if order.Side == types.SideUnspecified {
		return "side is required"
	}
	if order.Quantity == 0 {
		return "quantity must be greater than zero"
	}
	if order.OrderType == types.OrderTypeLimit && order.Price.IsZero() {
		return "limit orders must have a price"
	}
	if order.OrderType == types.OrderTypeMarket && !order.Price.IsZero() {
		return "market orders must not have a price"
	}
	if order.InstrumentID != "" && order.InstrumentID != ob.InstrumentID {
		return fmt.Sprintf("instrument mismatch: order has %s, book is %s", order.InstrumentID, ob.InstrumentID)
	}
	// Iceberg validation: only limit orders can be icebergs, and display must be < quantity
	if order.DisplayQty > 0 {
		if order.OrderType != types.OrderTypeLimit {
			return "iceberg orders must be limit orders"
		}
		if order.DisplayQty >= order.Quantity {
			return "iceberg display quantity must be less than total quantity"
		}
	}
	return ""
}

// match executes the price-time priority matching algorithm.
func (ob *OrderBook) match(incoming *types.Order, result *MatchResult, now time.Time) {
	oppositeLevels := ob.getOppositeLevels(incoming.Side)

	for incoming.RemainingQty > 0 && len(*oppositeLevels) > 0 {
		bestLevel := (*oppositeLevels)[0]

		// Price check for limit orders
		if incoming.OrderType == types.OrderTypeLimit {
			if !priceCrosses(incoming.Price, bestLevel.Price, incoming.Side) {
				break
			}
		}

		// Match against orders at this price level
		for incoming.RemainingQty > 0 && !bestLevel.IsEmpty() {
			resting := bestLevel.Front()

			// Self-trade prevention
			if resting.AccountID == incoming.AccountID && incoming.AccountID != "" {
				stpAction := ob.handleSTP(incoming, resting, bestLevel, result, now)
				if stpAction == stpCancelIncoming {
					return
				}
				if stpAction == stpCancelResting {
					continue // Try next resting order
				}
				// stpCancelBoth: incoming is already cancelled, return
				if stpAction == stpCancelBoth {
					return
				}
			}

			fillQty := incoming.RemainingQty
			if resting.RemainingQty < fillQty {
				fillQty = resting.RemainingQty
			}
			fillPrice := resting.Price // Resting order's price (price improvement for incoming)

			// Execute the fill
			incoming.Fill(fillQty)
			resting.Fill(fillQty)
			bestLevel.ReduceQty(fillQty)

			tradeSeq := ob.nextTradeSequence()
			tradeID := ob.idGen.NewID()

			// Determine buy/sell order IDs
			var buyOrderID, sellOrderID, buyerPID, sellerPID string
			if incoming.Side == types.SideBuy {
				buyOrderID = incoming.OrderID
				sellOrderID = resting.OrderID
				buyerPID = incoming.ParticipantID
				sellerPID = resting.ParticipantID
			} else {
				buyOrderID = resting.OrderID
				sellOrderID = incoming.OrderID
				buyerPID = resting.ParticipantID
				sellerPID = incoming.ParticipantID
			}

			trade := types.Trade{
				TradeID:             tradeID,
				InstrumentID:        ob.InstrumentID,
				BuyOrderID:          buyOrderID,
				SellOrderID:         sellOrderID,
				BuyerParticipantID:  buyerPID,
				SellerParticipantID: sellerPID,
				Price:               fillPrice,
				Quantity:            fillQty,
				TradeValue:          fillPrice.MulUint64(fillQty),
				AggressorSide:       incoming.Side,
				TradeType:           types.TradeTypeContinuous,
				SequenceNumber:      tradeSeq,
				ExecutedAt:          now,
			}
			result.Trades = append(result.Trades, trade)

			ob.LastTradePrice = fillPrice

			// Execution reports for both sides
			// Incoming order report
			incomingExecType := types.ExecTypePartialFill
			if incoming.RemainingQty == 0 {
				incomingExecType = types.ExecTypeFill
			}
			result.ExecutionReports = append(result.ExecutionReports, types.ExecutionReport{
				ExecID:        ob.idGen.NewID(),
				OrderID:       incoming.OrderID,
				ClientOrderID: incoming.ClientOrderID,
				ExecType:      incomingExecType,
				OrderStatus:   incoming.Status,
				Side:          incoming.Side,
				InstrumentID:  ob.InstrumentID,
				Price:         incoming.Price,
				Quantity:      incoming.Quantity,
				LastQty:       fillQty,
				LastPrice:     fillPrice,
				CumulativeQty: incoming.FilledQty,
				LeavesQty:     incoming.RemainingQty,
				TradeID:       tradeID,
				TransactTime:  now,
				AccountID:     incoming.AccountID,
			})

			// Resting order report
			// For iceberg orders, only report FILL when entire iceberg is exhausted
			restingExecType := types.ExecTypePartialFill
			if resting.RemainingQty == 0 && resting.HiddenQty == 0 {
				restingExecType = types.ExecTypeFill
			}
			result.ExecutionReports = append(result.ExecutionReports, types.ExecutionReport{
				ExecID:        ob.idGen.NewID(),
				OrderID:       resting.OrderID,
				ClientOrderID: resting.ClientOrderID,
				ExecType:      restingExecType,
				OrderStatus:   resting.Status,
				Side:          resting.Side,
				InstrumentID:  ob.InstrumentID,
				Price:         resting.Price,
				Quantity:      resting.Quantity,
				LastQty:       fillQty,
				LastPrice:     fillPrice,
				CumulativeQty: resting.FilledQty,
				LeavesQty:     resting.RemainingQty,
				TradeID:       tradeID,
				TransactTime:  now,
				AccountID:     resting.AccountID,
			})

			// Remove filled resting order from book, or replenish if iceberg
			if resting.RemainingQty == 0 {
				if ob.replenishIceberg(resting, bestLevel, now) {
					// Iceberg was replenished: order stays on book with new time priority.
					// Do NOT remove from orderIndex or orderLevelIndex.
				} else {
					bestLevel.Dequeue()
					delete(ob.orderIndex, resting.OrderID)
					delete(ob.orderLevelIndex, resting.OrderID)
				}
			}
		}

		// Remove empty price level
		if bestLevel.IsEmpty() {
			*oppositeLevels = (*oppositeLevels)[1:]
		}
	}
}

type stpResult int

const (
	stpNoAction       stpResult = 0
	stpCancelIncoming stpResult = 1
	stpCancelOldest   stpResult = 2
	stpCancelResting  stpResult = 2 // alias
	stpCancelBoth     stpResult = 3
)

// handleSTP processes self-trade prevention.
func (ob *OrderBook) handleSTP(incoming, resting *types.Order, level *PriceLevel, result *MatchResult, now time.Time) stpResult {
	mode := incoming.STPMode
	if mode == types.STPModeUnspecified {
		mode = types.STPModeCancelNewest // Default
	}

	switch mode {
	case types.STPModeCancelNewest:
		incoming.Status = types.OrderStatusCancelled
		incoming.RemainingQty = 0
		result.ExecutionReports = append(result.ExecutionReports, types.ExecutionReport{
			ExecID:        ob.idGen.NewID(),
			OrderID:       incoming.OrderID,
			ClientOrderID: incoming.ClientOrderID,
			ExecType:      types.ExecTypeCancelled,
			OrderStatus:   types.OrderStatusCancelled,
			Side:          incoming.Side,
			InstrumentID:  ob.InstrumentID,
			Price:         incoming.Price,
			Quantity:      incoming.Quantity,
			CumulativeQty: incoming.FilledQty,
			LeavesQty:     0,
			TransactTime:  now,
			RejectReason:  "self-trade prevention",
			AccountID:     incoming.AccountID,
		})
		return stpCancelIncoming

	case types.STPModeCancelOldest:
		level.RemoveOrder(resting.OrderID)
		resting.Status = types.OrderStatusCancelled
		delete(ob.orderIndex, resting.OrderID)
		delete(ob.orderLevelIndex, resting.OrderID)
		result.ExecutionReports = append(result.ExecutionReports, types.ExecutionReport{
			ExecID:        ob.idGen.NewID(),
			OrderID:       resting.OrderID,
			ClientOrderID: resting.ClientOrderID,
			ExecType:      types.ExecTypeCancelled,
			OrderStatus:   types.OrderStatusCancelled,
			Side:          resting.Side,
			InstrumentID:  ob.InstrumentID,
			Price:         resting.Price,
			Quantity:      resting.Quantity,
			CumulativeQty: resting.FilledQty,
			LeavesQty:     0,
			TransactTime:  now,
			RejectReason:  "self-trade prevention",
			AccountID:     resting.AccountID,
		})
		// Don't remove the empty level here — the outer match loop handles it
		return stpCancelResting

	case types.STPModeCancelBoth:
		// Cancel resting
		level.RemoveOrder(resting.OrderID)
		resting.Status = types.OrderStatusCancelled
		delete(ob.orderIndex, resting.OrderID)
		delete(ob.orderLevelIndex, resting.OrderID)
		result.ExecutionReports = append(result.ExecutionReports, types.ExecutionReport{
			ExecID:        ob.idGen.NewID(),
			OrderID:       resting.OrderID,
			ClientOrderID: resting.ClientOrderID,
			ExecType:      types.ExecTypeCancelled,
			OrderStatus:   types.OrderStatusCancelled,
			Side:          resting.Side,
			InstrumentID:  ob.InstrumentID,
			Price:         resting.Price,
			Quantity:      resting.Quantity,
			CumulativeQty: resting.FilledQty,
			LeavesQty:     0,
			TransactTime:  now,
			RejectReason:  "self-trade prevention",
			AccountID:     resting.AccountID,
		})
		// Cancel incoming
		incoming.Status = types.OrderStatusCancelled
		incoming.RemainingQty = 0
		result.ExecutionReports = append(result.ExecutionReports, types.ExecutionReport{
			ExecID:        ob.idGen.NewID(),
			OrderID:       incoming.OrderID,
			ClientOrderID: incoming.ClientOrderID,
			ExecType:      types.ExecTypeCancelled,
			OrderStatus:   types.OrderStatusCancelled,
			Side:          incoming.Side,
			InstrumentID:  ob.InstrumentID,
			Price:         incoming.Price,
			Quantity:      incoming.Quantity,
			CumulativeQty: incoming.FilledQty,
			LeavesQty:     0,
			TransactTime:  now,
			RejectReason:  "self-trade prevention",
			AccountID:     incoming.AccountID,
		})
		return stpCancelBoth
	}

	return stpNoAction
}

// canFillFOK checks if a FOK order can be fully filled.
func (ob *OrderBook) canFillFOK(order *types.Order) bool {
	var available uint64
	levels := ob.getOppositeLevelsReadOnly(order.Side)

	for _, level := range levels {
		if order.OrderType == types.OrderTypeLimit {
			if !priceCrosses(order.Price, level.Price, order.Side) {
				break
			}
		}
		available += level.TotalQty
		if available >= order.Quantity {
			return true
		}
	}
	return false
}

// priceCrosses checks if the incoming price would match against the resting price.
func priceCrosses(incomingPrice, restingPrice types.Decimal, side types.Side) bool {
	if side == types.SideBuy {
		return incomingPrice.GreaterThanOrEqual(restingPrice)
	}
	return incomingPrice.LessThanOrEqual(restingPrice)
}

// addToBook adds a resting order to the appropriate side of the book.
func (ob *OrderBook) addToBook(order *types.Order) {
	var levels *[]*PriceLevel
	if order.Side == types.SideBuy {
		levels = &ob.bids
	} else {
		levels = &ob.asks
	}

	// Find or create the price level
	level := ob.findOrCreateLevel(levels, order.Price, order.Side)
	level.Enqueue(order)

	ob.orderIndex[order.OrderID] = order
	ob.orderLevelIndex[order.OrderID] = level
}

// findOrCreateLevel finds an existing price level or creates a new one in sorted position.
func (ob *OrderBook) findOrCreateLevel(levels *[]*PriceLevel, price types.Decimal, side types.Side) *PriceLevel {
	// Binary search for the price level
	idx := sort.Search(len(*levels), func(i int) bool {
		if side == types.SideBuy {
			// Bids: descending. We want the first level with price <= target.
			return (*levels)[i].Price.LessThanOrEqual(price)
		}
		// Asks: ascending. We want the first level with price >= target.
		return (*levels)[i].Price.GreaterThanOrEqual(price)
	})

	// Check if level already exists at this price
	if idx < len(*levels) && (*levels)[idx].Price.Equal(price) {
		return (*levels)[idx]
	}

	// Insert new level at the correct position
	newLevel := NewPriceLevel(price)
	*levels = append(*levels, nil)
	copy((*levels)[idx+1:], (*levels)[idx:])
	(*levels)[idx] = newLevel
	return newLevel
}

// getOppositeLevels returns a mutable pointer to the opposite side's levels.
func (ob *OrderBook) getOppositeLevels(side types.Side) *[]*PriceLevel {
	if side == types.SideBuy {
		return &ob.asks
	}
	return &ob.bids
}

// getOppositeLevelsReadOnly returns the opposite side's levels for read-only access.
func (ob *OrderBook) getOppositeLevelsReadOnly(side types.Side) []*PriceLevel {
	if side == types.SideBuy {
		return ob.asks
	}
	return ob.bids
}

// removePriceLevel removes a price level from the given side.
func (ob *OrderBook) removePriceLevel(side types.Side, price types.Decimal) {
	var levels *[]*PriceLevel
	if side == types.SideBuy {
		levels = &ob.bids
	} else {
		levels = &ob.asks
	}

	for i, level := range *levels {
		if level.Price.Equal(price) {
			*levels = append((*levels)[:i], (*levels)[i+1:]...)
			return
		}
	}
}
