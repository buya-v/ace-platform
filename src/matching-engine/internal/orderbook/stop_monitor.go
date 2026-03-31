package orderbook

import (
	"github.com/garudax-platform/matching-engine/internal/types"
)

// StopMonitor manages stop orders that sit outside the main book and are
// triggered when the last trade price crosses their stop price.
// Stop orders are stored in a separate map and do not appear on the book
// until triggered. Once triggered, a stop-limit becomes a limit order and
// a stop-market becomes a market order, and both are submitted to the book.
//
// StopMonitor is designed to run AFTER the match completes so it does not
// affect matching latency for the hot path.
type StopMonitor struct {
	book       *OrderBook
	stopOrders map[string]*types.Order // orderID -> stop order
}

// NewStopMonitor creates a StopMonitor for the given order book.
func NewStopMonitor(book *OrderBook) *StopMonitor {
	return &StopMonitor{
		book:       book,
		stopOrders: make(map[string]*types.Order),
	}
}

// AddStopOrder adds a stop order to the monitor. The order is validated
// but not placed on the book. It will be triggered when OnTrade detects
// the last trade price crossing the stop price.
// Returns a NEW execution report for the accepted stop, or a REJECTED report.
func (sm *StopMonitor) AddStopOrder(order *types.Order) types.ExecutionReport {
	// Validate
	if order.OrderType != types.OrderTypeStopLimit && order.OrderType != types.OrderTypeStopMarket {
		return types.ExecutionReport{
			ExecID:       sm.book.idGen.NewID(),
			OrderID:      order.OrderID,
			ExecType:     types.ExecTypeRejected,
			OrderStatus:  types.OrderStatusRejected,
			Side:         order.Side,
			InstrumentID: sm.book.InstrumentID,
			Price:        order.Price,
			Quantity:     order.Quantity,
			LeavesQty:    order.Quantity,
			TransactTime: order.CreatedAt,
			RejectReason: "order type must be STOP_LIMIT or STOP_MARKET",
			AccountID:    order.AccountID,
		}
	}
	if order.StopPrice.IsZero() {
		return types.ExecutionReport{
			ExecID:       sm.book.idGen.NewID(),
			OrderID:      order.OrderID,
			ExecType:     types.ExecTypeRejected,
			OrderStatus:  types.OrderStatusRejected,
			Side:         order.Side,
			InstrumentID: sm.book.InstrumentID,
			Price:        order.Price,
			Quantity:     order.Quantity,
			LeavesQty:    order.Quantity,
			TransactTime: order.CreatedAt,
			RejectReason: "stop orders must have a stop price",
			AccountID:    order.AccountID,
		}
	}
	if order.Quantity == 0 {
		return types.ExecutionReport{
			ExecID:       sm.book.idGen.NewID(),
			OrderID:      order.OrderID,
			ExecType:     types.ExecTypeRejected,
			OrderStatus:  types.OrderStatusRejected,
			Side:         order.Side,
			InstrumentID: sm.book.InstrumentID,
			Price:        order.Price,
			Quantity:     order.Quantity,
			LeavesQty:    0,
			TransactTime: order.CreatedAt,
			RejectReason: "quantity must be greater than zero",
			AccountID:    order.AccountID,
		}
	}
	if order.OrderType == types.OrderTypeStopLimit && order.Price.IsZero() {
		return types.ExecutionReport{
			ExecID:       sm.book.idGen.NewID(),
			OrderID:      order.OrderID,
			ExecType:     types.ExecTypeRejected,
			OrderStatus:  types.OrderStatusRejected,
			Side:         order.Side,
			InstrumentID: sm.book.InstrumentID,
			Price:        order.Price,
			Quantity:     order.Quantity,
			LeavesQty:    order.Quantity,
			TransactTime: order.CreatedAt,
			RejectReason: "stop-limit orders must have a limit price",
			AccountID:    order.AccountID,
		}
	}

	order.Status = types.OrderStatusPendingNew
	order.RemainingQty = order.Quantity
	order.SequenceNumber = sm.book.nextSequence()
	sm.stopOrders[order.OrderID] = order

	return types.ExecutionReport{
		ExecID:        sm.book.idGen.NewID(),
		OrderID:       order.OrderID,
		ClientOrderID: order.ClientOrderID,
		ExecType:      types.ExecTypeNew,
		OrderStatus:   types.OrderStatusPendingNew,
		Side:          order.Side,
		InstrumentID:  sm.book.InstrumentID,
		Price:         order.Price,
		Quantity:      order.Quantity,
		LeavesQty:     order.Quantity,
		TransactTime:  order.CreatedAt,
		AccountID:     order.AccountID,
	}
}

// CancelStopOrder cancels a pending stop order before it triggers.
func (sm *StopMonitor) CancelStopOrder(orderID string) (types.ExecutionReport, bool) {
	order, ok := sm.stopOrders[orderID]
	if !ok {
		return types.ExecutionReport{}, false
	}

	delete(sm.stopOrders, orderID)
	order.Status = types.OrderStatusCancelled

	return types.ExecutionReport{
		ExecID:        sm.book.idGen.NewID(),
		OrderID:       order.OrderID,
		ClientOrderID: order.ClientOrderID,
		ExecType:      types.ExecTypeCancelled,
		OrderStatus:   types.OrderStatusCancelled,
		Side:          order.Side,
		InstrumentID:  sm.book.InstrumentID,
		Price:         order.Price,
		Quantity:      order.Quantity,
		CumulativeQty: 0,
		LeavesQty:     0,
		TransactTime:  order.CreatedAt,
		AccountID:     order.AccountID,
	}, true
}

// OnTrade checks if the last trade price triggers any stop orders.
// For buy stops: triggered when lastPrice >= stopPrice (price rising)
// For sell stops: triggered when lastPrice <= stopPrice (price falling)
//
// Triggered stops are converted to limit or market orders and submitted
// to the book. Returns the combined match results from all triggered orders.
//
// This method should be called AFTER the main match completes so it does
// not add latency to the critical matching path.
func (sm *StopMonitor) OnTrade(lastPrice types.Decimal) MatchResult {
	combined := MatchResult{}

	// Collect triggered order IDs to avoid map mutation during iteration
	var triggered []string
	for id, order := range sm.stopOrders {
		if isStopTriggered(order, lastPrice) {
			triggered = append(triggered, id)
		}
	}

	for _, id := range triggered {
		order, ok := sm.stopOrders[id]
		if !ok {
			continue
		}
		delete(sm.stopOrders, id)

		// Convert stop to regular order
		converted := sm.convertStopOrder(order)

		// Submit to the book
		result := sm.book.SubmitOrder(converted)

		combined.Trades = append(combined.Trades, result.Trades...)
		combined.ExecutionReports = append(combined.ExecutionReports, result.ExecutionReports...)
	}

	return combined
}

// StopOrderCount returns the number of pending stop orders.
func (sm *StopMonitor) StopOrderCount() int {
	return len(sm.stopOrders)
}

// GetStopOrder returns a pending stop order by ID.
func (sm *StopMonitor) GetStopOrder(orderID string) (*types.Order, bool) {
	o, ok := sm.stopOrders[orderID]
	return o, ok
}

// isStopTriggered checks if a stop order should be triggered at the given price.
func isStopTriggered(order *types.Order, lastPrice types.Decimal) bool {
	switch order.Side {
	case types.SideBuy:
		// Buy stop triggers when price rises to or above stop price
		return lastPrice.GreaterThanOrEqual(order.StopPrice)
	case types.SideSell:
		// Sell stop triggers when price falls to or below stop price
		return lastPrice.LessThanOrEqual(order.StopPrice)
	}
	return false
}

// convertStopOrder converts a triggered stop order into a regular order.
func (sm *StopMonitor) convertStopOrder(stop *types.Order) *types.Order {
	converted := &types.Order{
		OrderID:       stop.OrderID,
		ClientOrderID: stop.ClientOrderID,
		InstrumentID:  stop.InstrumentID,
		AccountID:     stop.AccountID,
		ParticipantID: stop.ParticipantID,
		Side:          stop.Side,
		TimeInForce:   stop.TimeInForce,
		Quantity:      stop.Quantity,
		STPMode:       stop.STPMode,
		ExpireAt:      stop.ExpireAt,
		DisplayQty:    stop.DisplayQty,
	}

	switch stop.OrderType {
	case types.OrderTypeStopLimit:
		converted.OrderType = types.OrderTypeLimit
		converted.Price = stop.Price
	case types.OrderTypeStopMarket:
		converted.OrderType = types.OrderTypeMarket
		// Market orders have zero price
	}

	return converted
}
