package orderbook

import (
	"context"
	"sync"
	"time"

	"github.com/garudax-platform/matching-engine/internal/types"
)

// ExpiryCallback is invoked when an order expires.
// The callback receives the execution report for the expired order.
type ExpiryCallback func(report types.ExecutionReport)

// ExpiryChecker monitors GTD orders and cancels them when they expire.
// It runs a background goroutine that checks every second.
type ExpiryChecker struct {
	mu       sync.Mutex
	book     *OrderBook
	callback ExpiryCallback
	nowFunc  func() time.Time // injectable clock for testing
}

// NewExpiryChecker creates an ExpiryChecker for the given order book.
func NewExpiryChecker(book *OrderBook, callback ExpiryCallback) *ExpiryChecker {
	return &ExpiryChecker{
		book:     book,
		callback: callback,
		nowFunc:  time.Now,
	}
}

// StartExpiryChecker creates an ExpiryChecker and starts its background loop.
// The checker runs every second until ctx is cancelled.
// Returns the checker so tests can call ExpireOrders directly.
func StartExpiryChecker(ctx context.Context, book *OrderBook, callback ExpiryCallback) *ExpiryChecker {
	ec := NewExpiryChecker(book, callback)
	go ec.run(ctx)
	return ec
}

// run is the background loop that checks for expired orders every second.
func (ec *ExpiryChecker) run(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ec.ExpireOrders()
		}
	}
}

// ExpireOrders scans all resting orders on the book and cancels any GTD
// orders whose ExpireAt time has passed. This method is safe to call
// concurrently (it acquires a lock) but the book itself is single-threaded
// per instrument, so in production this should be called from the same
// goroutine that processes orders, or protected by the instrument lock.
func (ec *ExpiryChecker) ExpireOrders() []types.ExecutionReport {
	ec.mu.Lock()
	defer ec.mu.Unlock()

	now := ec.nowFunc()
	var reports []types.ExecutionReport

	// Collect expired order IDs first to avoid mutating the book during iteration
	var expiredIDs []string
	for id, order := range ec.book.orderIndex {
		if order.TimeInForce == types.TIFGTD && !order.ExpireAt.IsZero() && now.After(order.ExpireAt) {
			expiredIDs = append(expiredIDs, id)
		}
	}

	for _, id := range expiredIDs {
		order, ok := ec.book.orderIndex[id]
		if !ok {
			continue // already removed (race with cancel)
		}

		// Remove from price level
		level, ok := ec.book.orderLevelIndex[id]
		if ok {
			level.RemoveOrder(id)
			if level.IsEmpty() {
				ec.book.removePriceLevel(order.Side, level.Price)
			}
		}

		delete(ec.book.orderIndex, id)
		delete(ec.book.orderLevelIndex, id)

		order.Status = types.OrderStatusExpired

		report := types.ExecutionReport{
			ExecID:        ec.book.idGen.NewID(),
			OrderID:       order.OrderID,
			ClientOrderID: order.ClientOrderID,
			ExecType:      types.ExecTypeExpired,
			OrderStatus:   types.OrderStatusExpired,
			Side:          order.Side,
			InstrumentID:  ec.book.InstrumentID,
			Price:         order.Price,
			Quantity:      order.Quantity,
			CumulativeQty: order.FilledQty,
			LeavesQty:     0,
			TransactTime:  now,
			AccountID:     order.AccountID,
		}
		reports = append(reports, report)

		if ec.callback != nil {
			ec.callback(report)
		}
	}

	return reports
}
