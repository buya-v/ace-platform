package margincall

import (
	"sync"
	"testing"
	"time"

	"github.com/garudax-platform/margin-engine/internal/types"
)

// TestEvaluateHandlerRunsOutsideLock verifies that the margin-call handler is
// invoked after s.mu is released. The handler re-enters the service via
// GetActive (which acquires s.mu); if the handler ran inside the critical
// section this would deadlock on the non-reentrant mutex.
func TestEvaluateHandlerRunsOutsideLock(t *testing.T) {
	svc := NewService(&testIDGen{}, 1*time.Hour)

	svc.SetHandler(func(call types.MarginCall) {
		// Re-enter the service — must not deadlock now that the handler runs
		// outside the lock.
		if _, ok := svc.GetActive(call.ParticipantID); !ok {
			t.Error("active call not found from within handler")
		}
	})

	done := make(chan struct{})
	go func() {
		svc.Evaluate(deficitPortfolio("P1", 100000, 50000))
		close(done)
	}()

	select {
	case <-done:
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("Evaluate deadlocked — handler is still being invoked inside the lock")
	}
}

// TestEvaluateConcurrentRace exercises Evaluate, SetHandler, and the read
// accessors concurrently. Run with -race to catch unsynchronized access to the
// handler field or the calls/active maps.
func TestEvaluateConcurrentRace(t *testing.T) {
	svc := NewService(&testIDGen{}, 1*time.Hour)

	const workers = 16
	var wg sync.WaitGroup
	wg.Add(workers)

	for w := 0; w < workers; w++ {
		go func(w int) {
			defer wg.Done()
			pid := participantID(w)
			for i := 0; i < 25; i++ {
				// Alternate deficit/surplus to exercise issue + resolve paths.
				if i%2 == 0 {
					svc.Evaluate(deficitPortfolio(pid, 100000, 50000))
				} else {
					svc.Evaluate(surplusPortfolio(pid))
				}
			}
		}(w)
	}

	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			svc.SetHandler(func(_ types.MarginCall) {})
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			_ = svc.GetAllActive()
		}
	}()

	wg.Wait()
}

func participantID(w int) string {
	return "P" + string(rune('A'+w))
}
