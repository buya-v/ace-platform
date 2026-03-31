package orderbook

import (
	"context"
	"testing"
	"time"

	"github.com/garudax-platform/matching-engine/internal/types"
)

func TestExpiryCheckerExpiresGTDOrder(t *testing.T) {
	book := newTestBook()

	// Place a GTD order that expires in the past
	order := newLimitOrder("gtd1", types.SideBuy, "100.00", 10)
	order.TimeInForce = types.TIFGTD
	order.ExpireAt = time.Now().Add(-1 * time.Second) // Already expired
	book.SubmitOrder(order)

	if book.OrderCount() != 1 {
		t.Fatalf("expected 1 order on book, got %d", book.OrderCount())
	}

	var reports []types.ExecutionReport
	callback := func(r types.ExecutionReport) {
		reports = append(reports, r)
	}

	ec := NewExpiryChecker(book, callback)
	expired := ec.ExpireOrders()

	if len(expired) != 1 {
		t.Fatalf("expected 1 expired order, got %d", len(expired))
	}
	if expired[0].OrderID != "gtd1" {
		t.Errorf("expected expired order gtd1, got %s", expired[0].OrderID)
	}
	if expired[0].ExecType != types.ExecTypeExpired {
		t.Errorf("expected EXPIRED exec type, got %d", expired[0].ExecType)
	}
	if expired[0].OrderStatus != types.OrderStatusExpired {
		t.Errorf("expected EXPIRED status, got %s", expired[0].OrderStatus.String())
	}

	if book.OrderCount() != 0 {
		t.Errorf("expected empty book after expiry, got %d", book.OrderCount())
	}

	// Callback should have been invoked
	if len(reports) != 1 {
		t.Errorf("expected 1 callback, got %d", len(reports))
	}
}

func TestExpiryCheckerDoesNotExpireNonGTD(t *testing.T) {
	book := newTestBook()

	// GTC order should never expire
	gtc := newLimitOrder("gtc1", types.SideBuy, "100.00", 10)
	gtc.TimeInForce = types.TIFGTC
	book.SubmitOrder(gtc)

	// DAY order without GTD TIF should not be expired by checker
	day := newLimitOrder("day1", types.SideSell, "101.00", 10)
	day.TimeInForce = types.TIFDay
	book.SubmitOrder(day)

	ec := NewExpiryChecker(book, nil)
	expired := ec.ExpireOrders()

	if len(expired) != 0 {
		t.Errorf("expected 0 expired orders, got %d", len(expired))
	}
	if book.OrderCount() != 2 {
		t.Errorf("expected 2 orders still on book, got %d", book.OrderCount())
	}
}

func TestExpiryCheckerDoesNotExpireFutureGTD(t *testing.T) {
	book := newTestBook()

	// GTD order that expires in the future
	order := newLimitOrder("gtd-future", types.SideBuy, "100.00", 10)
	order.TimeInForce = types.TIFGTD
	order.ExpireAt = time.Now().Add(1 * time.Hour)
	book.SubmitOrder(order)

	ec := NewExpiryChecker(book, nil)
	expired := ec.ExpireOrders()

	if len(expired) != 0 {
		t.Errorf("expected 0 expired (future), got %d", len(expired))
	}
	if book.OrderCount() != 1 {
		t.Errorf("expected 1 order still on book, got %d", book.OrderCount())
	}
}

func TestExpiryCheckerRemovesPriceLevel(t *testing.T) {
	book := newTestBook()

	// Single order at a price level, expires
	order := newLimitOrder("gtd1", types.SideSell, "105.00", 10)
	order.TimeInForce = types.TIFGTD
	order.ExpireAt = time.Now().Add(-1 * time.Second)
	book.SubmitOrder(order)

	if len(book.AskLevels()) != 1 {
		t.Fatalf("expected 1 ask level, got %d", len(book.AskLevels()))
	}

	ec := NewExpiryChecker(book, nil)
	ec.ExpireOrders()

	if len(book.AskLevels()) != 0 {
		t.Errorf("expected 0 ask levels after expiry, got %d", len(book.AskLevels()))
	}
}

func TestExpiryCheckerPartiallyFilledOrder(t *testing.T) {
	book := newTestBook()

	// Place GTD sell that expires soon
	sell := newLimitOrder("gtd-sell", types.SideSell, "100.00", 10)
	sell.TimeInForce = types.TIFGTD
	sell.ExpireAt = time.Now().Add(100 * time.Millisecond)
	book.SubmitOrder(sell)

	// Partially fill it
	buy := newLimitOrder("buy1", types.SideBuy, "100.00", 3)
	book.SubmitOrder(buy)

	o, _ := book.GetOrder("gtd-sell")
	if o.RemainingQty != 7 {
		t.Fatalf("expected 7 remaining, got %d", o.RemainingQty)
	}

	// Now expire it (use custom clock to force expiry)
	ec := NewExpiryChecker(book, nil)
	ec.nowFunc = func() time.Time { return time.Now().Add(1 * time.Second) }
	expired := ec.ExpireOrders()

	if len(expired) != 1 {
		t.Fatalf("expected 1 expired, got %d", len(expired))
	}
	if expired[0].CumulativeQty != 3 {
		t.Errorf("expected cumulative qty 3, got %d", expired[0].CumulativeQty)
	}
	if book.OrderCount() != 0 {
		t.Errorf("expected empty book, got %d", book.OrderCount())
	}
}

func TestExpiryCheckerMultipleOrders(t *testing.T) {
	book := newTestBook()

	// Two expired GTD orders + one non-expired
	expired1 := newLimitOrder("exp1", types.SideBuy, "99.00", 10)
	expired1.TimeInForce = types.TIFGTD
	expired1.ExpireAt = time.Now().Add(-2 * time.Second)
	book.SubmitOrder(expired1)

	expired2 := newLimitOrder("exp2", types.SideSell, "101.00", 5)
	expired2.TimeInForce = types.TIFGTD
	expired2.ExpireAt = time.Now().Add(-1 * time.Second)
	book.SubmitOrder(expired2)

	active := newLimitOrder("active", types.SideBuy, "98.00", 10)
	active.TimeInForce = types.TIFGTD
	active.ExpireAt = time.Now().Add(1 * time.Hour)
	book.SubmitOrder(active)

	ec := NewExpiryChecker(book, nil)
	reports := ec.ExpireOrders()

	if len(reports) != 2 {
		t.Fatalf("expected 2 expired, got %d", len(reports))
	}
	if book.OrderCount() != 1 {
		t.Errorf("expected 1 order remaining, got %d", book.OrderCount())
	}
}

func TestStartExpiryCheckerStopsOnCancel(t *testing.T) {
	book := newTestBook()

	ctx, cancel := context.WithCancel(context.Background())

	var callbackCount int
	ec := StartExpiryChecker(ctx, book, func(r types.ExecutionReport) {
		callbackCount++
	})
	_ = ec

	// Cancel immediately — the goroutine should exit
	cancel()

	// Give goroutine time to exit
	time.Sleep(50 * time.Millisecond)

	// No panic or goroutine leak = pass
}

func TestExpiryCheckerWithNilCallback(t *testing.T) {
	book := newTestBook()

	order := newLimitOrder("gtd1", types.SideBuy, "100.00", 10)
	order.TimeInForce = types.TIFGTD
	order.ExpireAt = time.Now().Add(-1 * time.Second)
	book.SubmitOrder(order)

	// nil callback should not panic
	ec := NewExpiryChecker(book, nil)
	expired := ec.ExpireOrders()

	if len(expired) != 1 {
		t.Errorf("expected 1 expired, got %d", len(expired))
	}
}
