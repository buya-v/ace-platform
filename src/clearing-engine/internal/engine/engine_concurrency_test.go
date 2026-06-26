package engine

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/garudax-platform/clearing-engine/internal/novation"
	"github.com/garudax-platform/clearing-engine/internal/types"
)

// TestClearTradeHandlerRunsOutsideLock verifies that the trade handler is
// invoked after e.mu is released. The handler re-enters the engine via
// NetObligations (which acquires e.mu); if the handler ran inside the critical
// section this would deadlock on the non-reentrant mutex.
func TestClearTradeHandlerRunsOutsideLock(t *testing.T) {
	eng := newTestEngine()

	eng.SetTradeHandler(func(_ types.Trade, _ novation.NovationResult) {
		// Re-enter the engine — must not deadlock now that the handler runs
		// outside the lock.
		_ = eng.NetObligations()
	})

	done := make(chan struct{})
	go func() {
		eng.ClearTrade(makeTrade("t-1", "buyer-1", "seller-1", 500, 10))
		close(done)
	}()

	select {
	case <-done:
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("ClearTrade deadlocked — handler is still being invoked inside the lock")
	}
}

// TestClearTradeConcurrentRace exercises ClearTrade, SetTradeHandler, and the
// read accessors concurrently. Run with -race to catch unsynchronized access
// to the handler field or the processedTrades map.
func TestClearTradeConcurrentRace(t *testing.T) {
	eng := newTestEngine()

	const workers = 16
	var wg sync.WaitGroup
	wg.Add(workers)

	for w := 0; w < workers; w++ {
		go func(w int) {
			defer wg.Done()
			for i := 0; i < 25; i++ {
				id := fmt.Sprintf("t-%d-%d", w, i)
				eng.ClearTrade(makeTrade(id, "buyer-1", "seller-1", 500, 1))
			}
		}(w)
	}

	// Concurrently mutate the handler and read positions.
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			eng.SetTradeHandler(func(_ types.Trade, _ novation.NovationResult) {})
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			_ = eng.GetPositions("buyer-1")
		}
	}()

	wg.Wait()
}
