package engine

import (
	"fmt"
	"sort"
	"sync/atomic"
	"time"
)

// ---------------------------------------------------------------------------
// PriceLevel
// ---------------------------------------------------------------------------

type PriceLevel struct {
	Price      Decimal
	orders     []*Order
	TotalQty   uint64
	OrderCount uint32
}

func NewPriceLevel(price Decimal) *PriceLevel {
	return &PriceLevel{
		Price:  price,
		orders: make([]*Order, 0, 4),
	}
}

func (pl *PriceLevel) Enqueue(order *Order)  { pl.orders = append(pl.orders, order); pl.TotalQty += order.RemainingQty; pl.OrderCount++ }
func (pl *PriceLevel) Front() *Order         { if len(pl.orders) == 0 { return nil }; return pl.orders[0] }
func (pl *PriceLevel) IsEmpty() bool         { return len(pl.orders) == 0 }
func (pl *PriceLevel) ReduceQty(qty uint64)  { pl.TotalQty -= qty }

func (pl *PriceLevel) Dequeue() *Order {
	if len(pl.orders) == 0 {
		return nil
	}
	o := pl.orders[0]
	pl.orders = pl.orders[1:]
	pl.TotalQty -= o.RemainingQty
	pl.OrderCount--
	return o
}

func (pl *PriceLevel) RemoveOrder(orderID string) *Order {
	for i, o := range pl.orders {
		if o.OrderID == orderID {
			pl.orders = append(pl.orders[:i], pl.orders[i+1:]...)
			pl.TotalQty -= o.RemainingQty
			pl.OrderCount--
			return o
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// OrderBook — Central Limit Order Book with price-time priority
// ---------------------------------------------------------------------------

type MatchResult struct {
	Trades           []Trade
	ExecutionReports []ExecutionReport
}

type OrderBook struct {
	InstrumentID   string
	State          BookState
	LastTradePrice Decimal

	bids []*PriceLevel
	asks []*PriceLevel

	orderIndex      map[string]*Order
	orderLevelIndex map[string]*PriceLevel

	sequenceCounter uint64
	idGen           IDGenerator
	globalSeq       *uint64
}

func NewOrderBook(instrumentID string, idGen IDGenerator, globalSeq *uint64) *OrderBook {
	return &OrderBook{
		InstrumentID:    instrumentID,
		State:           BookStateContinuous,
		bids:            make([]*PriceLevel, 0),
		asks:            make([]*PriceLevel, 0),
		orderIndex:      make(map[string]*Order),
		orderLevelIndex: make(map[string]*PriceLevel),
		idGen:           idGen,
		globalSeq:       globalSeq,
	}
}

func (ob *OrderBook) nextSequence() uint64      { return atomic.AddUint64(ob.globalSeq, 1) }
func (ob *OrderBook) nextTradeSequence() uint64  { ob.sequenceCounter++; return ob.sequenceCounter }
func (ob *OrderBook) OrderCount() int            { return len(ob.orderIndex) }
func (ob *OrderBook) BestBid() Decimal {
	if len(ob.bids) == 0 { return DecimalZero() }
	return ob.bids[0].Price
}
func (ob *OrderBook) BestAsk() Decimal {
	if len(ob.asks) == 0 { return DecimalZero() }
	return ob.asks[0].Price
}

func (ob *OrderBook) SubmitOrder(order *Order) MatchResult {
	result := MatchResult{}
	now := time.Now()

	order.SequenceNumber = ob.nextSequence()
	order.CreatedAt = now

	if reason := ob.validateOrder(order); reason != "" {
		order.Status = OrderStatusRejected
		result.ExecutionReports = append(result.ExecutionReports, ExecutionReport{
			ExecID: ob.idGen.NewID(), OrderID: order.OrderID, ExecType: ExecTypeRejected,
			OrderStatus: OrderStatusRejected, Side: order.Side, InstrumentID: ob.InstrumentID,
			Price: order.Price, Quantity: order.Quantity, LeavesQty: order.Quantity,
			TransactTime: now, RejectReason: reason, AccountID: order.AccountID,
		})
		return result
	}

	order.Status = OrderStatusNew
	order.RemainingQty = order.Quantity

	result.ExecutionReports = append(result.ExecutionReports, ExecutionReport{
		ExecID: ob.idGen.NewID(), OrderID: order.OrderID, ExecType: ExecTypeNew,
		OrderStatus: OrderStatusNew, Side: order.Side, InstrumentID: ob.InstrumentID,
		Price: order.Price, Quantity: order.Quantity, LeavesQty: order.RemainingQty,
		TransactTime: now, AccountID: order.AccountID,
	})

	ob.match(order, &result, now)

	if order.RemainingQty > 0 {
		if order.OrderType == OrderTypeMarket || order.TimeInForce == TIFIOC {
			order.Status = OrderStatusCancelled
			result.ExecutionReports = append(result.ExecutionReports, ExecutionReport{
				ExecID: ob.idGen.NewID(), OrderID: order.OrderID, ExecType: ExecTypeCancelled,
				OrderStatus: OrderStatusCancelled, Side: order.Side, InstrumentID: ob.InstrumentID,
				Price: order.Price, Quantity: order.Quantity, CumulativeQty: order.FilledQty,
				LeavesQty: 0, TransactTime: now, AccountID: order.AccountID,
			})
		} else {
			ob.addToBook(order)
		}
	}

	return result
}

func (ob *OrderBook) CancelOrder(orderID string) (ExecutionReport, error) {
	order, ok := ob.orderIndex[orderID]
	if !ok {
		return ExecutionReport{}, fmt.Errorf("order %s not found", orderID)
	}
	level, ok := ob.orderLevelIndex[orderID]
	if ok {
		level.RemoveOrder(orderID)
		if level.IsEmpty() {
			ob.removePriceLevel(order.Side, level.Price)
		}
	}
	delete(ob.orderIndex, orderID)
	delete(ob.orderLevelIndex, orderID)
	order.Status = OrderStatusCancelled
	now := time.Now()
	return ExecutionReport{
		ExecID: ob.idGen.NewID(), OrderID: order.OrderID, ExecType: ExecTypeCancelled,
		OrderStatus: OrderStatusCancelled, Side: order.Side, InstrumentID: ob.InstrumentID,
		Price: order.Price, Quantity: order.Quantity, CumulativeQty: order.FilledQty,
		LeavesQty: 0, TransactTime: now, AccountID: order.AccountID,
	}, nil
}

func (ob *OrderBook) validateOrder(order *Order) string {
	if ob.State != BookStateContinuous {
		return fmt.Sprintf("book state %d does not accept orders", ob.State)
	}
	if order.Side == SideUnspecified {
		return "side is required"
	}
	if order.Quantity == 0 {
		return "quantity must be greater than zero"
	}
	if order.OrderType == OrderTypeLimit && order.Price.IsZero() {
		return "limit orders must have a price"
	}
	if order.OrderType == OrderTypeMarket && !order.Price.IsZero() {
		return "market orders must not have a price"
	}
	if order.InstrumentID != "" && order.InstrumentID != ob.InstrumentID {
		return fmt.Sprintf("instrument mismatch: order has %s, book is %s", order.InstrumentID, ob.InstrumentID)
	}
	return ""
}

func (ob *OrderBook) match(incoming *Order, result *MatchResult, now time.Time) {
	oppositeLevels := ob.getOppositeLevels(incoming.Side)

	for incoming.RemainingQty > 0 && len(*oppositeLevels) > 0 {
		bestLevel := (*oppositeLevels)[0]

		if incoming.OrderType == OrderTypeLimit {
			if !priceCrosses(incoming.Price, bestLevel.Price, incoming.Side) {
				break
			}
		}

		for incoming.RemainingQty > 0 && !bestLevel.IsEmpty() {
			resting := bestLevel.Front()

			// Self-trade prevention (simplified for benchmarks)
			if resting.AccountID == incoming.AccountID && incoming.AccountID != "" {
				incoming.Status = OrderStatusCancelled
				incoming.RemainingQty = 0
				result.ExecutionReports = append(result.ExecutionReports, ExecutionReport{
					ExecID: ob.idGen.NewID(), OrderID: incoming.OrderID, ExecType: ExecTypeCancelled,
					OrderStatus: OrderStatusCancelled, Side: incoming.Side, InstrumentID: ob.InstrumentID,
					TransactTime: now, RejectReason: "self-trade prevention", AccountID: incoming.AccountID,
				})
				return
			}

			fillQty := incoming.RemainingQty
			if resting.RemainingQty < fillQty {
				fillQty = resting.RemainingQty
			}
			fillPrice := resting.Price

			incoming.Fill(fillQty)
			resting.Fill(fillQty)
			bestLevel.ReduceQty(fillQty)

			tradeSeq := ob.nextTradeSequence()
			tradeID := ob.idGen.NewID()

			var buyOrderID, sellOrderID, buyerPID, sellerPID string
			if incoming.Side == SideBuy {
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

			result.Trades = append(result.Trades, Trade{
				TradeID: tradeID, InstrumentID: ob.InstrumentID,
				BuyOrderID: buyOrderID, SellOrderID: sellOrderID,
				BuyerParticipantID: buyerPID, SellerParticipantID: sellerPID,
				Price: fillPrice, Quantity: fillQty, TradeValue: fillPrice.MulUint64(fillQty),
				AggressorSide: incoming.Side, TradeType: TradeTypeContinuous,
				SequenceNumber: tradeSeq, ExecutedAt: now,
			})

			ob.LastTradePrice = fillPrice

			inExecType := ExecTypePartialFill
			if incoming.RemainingQty == 0 {
				inExecType = ExecTypeFill
			}
			result.ExecutionReports = append(result.ExecutionReports, ExecutionReport{
				ExecID: ob.idGen.NewID(), OrderID: incoming.OrderID, ExecType: inExecType,
				OrderStatus: incoming.Status, Side: incoming.Side, InstrumentID: ob.InstrumentID,
				Price: incoming.Price, Quantity: incoming.Quantity, LastQty: fillQty,
				LastPrice: fillPrice, CumulativeQty: incoming.FilledQty,
				LeavesQty: incoming.RemainingQty, TradeID: tradeID, TransactTime: now,
				AccountID: incoming.AccountID,
			})

			rExecType := ExecTypePartialFill
			if resting.RemainingQty == 0 {
				rExecType = ExecTypeFill
			}
			result.ExecutionReports = append(result.ExecutionReports, ExecutionReport{
				ExecID: ob.idGen.NewID(), OrderID: resting.OrderID, ExecType: rExecType,
				OrderStatus: resting.Status, Side: resting.Side, InstrumentID: ob.InstrumentID,
				Price: resting.Price, Quantity: resting.Quantity, LastQty: fillQty,
				LastPrice: fillPrice, CumulativeQty: resting.FilledQty,
				LeavesQty: resting.RemainingQty, TradeID: tradeID, TransactTime: now,
				AccountID: resting.AccountID,
			})

			if resting.RemainingQty == 0 {
				bestLevel.Dequeue()
				delete(ob.orderIndex, resting.OrderID)
				delete(ob.orderLevelIndex, resting.OrderID)
			}
		}

		if bestLevel.IsEmpty() {
			*oppositeLevels = (*oppositeLevels)[1:]
		}
	}
}

func priceCrosses(incomingPrice, restingPrice Decimal, side Side) bool {
	if side == SideBuy {
		return incomingPrice.GreaterThanOrEqual(restingPrice)
	}
	return incomingPrice.LessThanOrEqual(restingPrice)
}

func (ob *OrderBook) addToBook(order *Order) {
	var levels *[]*PriceLevel
	if order.Side == SideBuy {
		levels = &ob.bids
	} else {
		levels = &ob.asks
	}
	level := ob.findOrCreateLevel(levels, order.Price, order.Side)
	level.Enqueue(order)
	ob.orderIndex[order.OrderID] = order
	ob.orderLevelIndex[order.OrderID] = level
}

func (ob *OrderBook) findOrCreateLevel(levels *[]*PriceLevel, price Decimal, side Side) *PriceLevel {
	idx := sort.Search(len(*levels), func(i int) bool {
		if side == SideBuy {
			return (*levels)[i].Price.LessThanOrEqual(price)
		}
		return (*levels)[i].Price.GreaterThanOrEqual(price)
	})
	if idx < len(*levels) && (*levels)[idx].Price.Equal(price) {
		return (*levels)[idx]
	}
	newLevel := NewPriceLevel(price)
	*levels = append(*levels, nil)
	copy((*levels)[idx+1:], (*levels)[idx:])
	(*levels)[idx] = newLevel
	return newLevel
}

func (ob *OrderBook) getOppositeLevels(side Side) *[]*PriceLevel {
	if side == SideBuy {
		return &ob.asks
	}
	return &ob.bids
}

func (ob *OrderBook) removePriceLevel(side Side, price Decimal) {
	var levels *[]*PriceLevel
	if side == SideBuy {
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
