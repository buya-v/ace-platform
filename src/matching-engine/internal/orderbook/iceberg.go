package orderbook

import (
	"time"

	"github.com/garudax-platform/matching-engine/internal/types"
)

// replenishIceberg checks if the filled resting order is an iceberg and
// replenishes its visible quantity from the hidden reserve. On replenishment,
// the order gets a new timestamp (loses time priority): it is removed from
// the front of the price level queue and re-enqueued at the back.
//
// Returns true if the order was replenished (caller should NOT remove it
// from the book), false if it is fully exhausted (caller should remove it).
func (ob *OrderBook) replenishIceberg(order *types.Order, level *PriceLevel, now time.Time) bool {
	if !order.IsIceberg() {
		return false
	}
	if order.HiddenQty == 0 {
		return false
	}

	// Calculate the next display slice
	replenishQty := order.DisplayQty
	if order.HiddenQty < replenishQty {
		replenishQty = order.HiddenQty
	}

	// Lose time priority: remove from current position BEFORE updating qty.
	// The order is currently at the front of the level (it was just matched).
	// RemainingQty is 0 at this point, so Dequeue subtracts 0 from level total.
	level.Dequeue()

	// Move quantity from hidden to visible
	order.HiddenQty -= replenishQty
	order.RemainingQty = replenishQty

	// Reset status since it has remaining quantity again
	if order.FilledQty > 0 {
		order.Status = types.OrderStatusPartiallyFilled
	} else {
		order.Status = types.OrderStatusNew
	}

	// Update the timestamp so it sorts after existing orders at same price
	order.CreatedAt = now
	order.SequenceNumber = ob.nextSequence()
	// Re-enqueue at back with new qty
	level.Enqueue(order)

	return true
}

// initIceberg initializes iceberg fields on order acceptance.
// Sets TotalQty, HiddenQty, and adjusts RemainingQty to DisplayQty.
func initIceberg(order *types.Order) {
	if order.DisplayQty == 0 || order.DisplayQty >= order.Quantity {
		// Not an iceberg or display >= total: treat as normal order
		order.DisplayQty = 0
		order.TotalQty = 0
		order.HiddenQty = 0
		return
	}

	order.TotalQty = order.Quantity
	order.HiddenQty = order.Quantity - order.DisplayQty
	order.RemainingQty = order.DisplayQty
}
