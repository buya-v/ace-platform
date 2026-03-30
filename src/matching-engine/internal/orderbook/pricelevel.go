package orderbook

import (
	"github.com/garudax-platform/matching-engine/internal/types"
)

// PriceLevel represents a single price level in the order book.
// Orders at the same price are queued in FIFO order (time priority).
type PriceLevel struct {
	Price      types.Decimal
	orders     []*types.Order // FIFO queue; front = oldest
	TotalQty   uint64
	OrderCount uint32
}

// NewPriceLevel creates a new price level.
func NewPriceLevel(price types.Decimal) *PriceLevel {
	return &PriceLevel{
		Price:  price,
		orders: make([]*types.Order, 0, 4),
	}
}

// Enqueue adds an order to the back of the queue.
func (pl *PriceLevel) Enqueue(order *types.Order) {
	pl.orders = append(pl.orders, order)
	pl.TotalQty += order.RemainingQty
	pl.OrderCount++
}

// Front returns the first order in the queue without removing it.
func (pl *PriceLevel) Front() *types.Order {
	if len(pl.orders) == 0 {
		return nil
	}
	return pl.orders[0]
}

// Dequeue removes the first order from the queue.
func (pl *PriceLevel) Dequeue() *types.Order {
	if len(pl.orders) == 0 {
		return nil
	}
	order := pl.orders[0]
	pl.orders = pl.orders[1:]
	pl.TotalQty -= order.RemainingQty
	pl.OrderCount--
	return order
}

// RemoveOrder removes a specific order by ID. Used for cancellations.
// Returns the removed order, or nil if not found.
func (pl *PriceLevel) RemoveOrder(orderID string) *types.Order {
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

// IsEmpty returns true if there are no orders at this level.
func (pl *PriceLevel) IsEmpty() bool {
	return len(pl.orders) == 0
}

// Orders returns a copy of the orders slice for inspection.
func (pl *PriceLevel) Orders() []*types.Order {
	result := make([]*types.Order, len(pl.orders))
	copy(result, pl.orders)
	return result
}

// Len returns the number of orders at this level.
func (pl *PriceLevel) Len() int {
	return len(pl.orders)
}

// ReduceQty reduces TotalQty by the given amount (called when an order is partially filled).
func (pl *PriceLevel) ReduceQty(qty uint64) {
	pl.TotalQty -= qty
}
