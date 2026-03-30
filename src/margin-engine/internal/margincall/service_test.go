package margincall

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/garudax-platform/margin-engine/internal/types"
)

type testIDGen struct {
	counter uint64
}

func (g *testIDGen) NewID() string {
	n := atomic.AddUint64(&g.counter, 1)
	return fmt.Sprintf("test-mc-%d", n)
}

func deficitPortfolio(participantID string, required, onHand int64) types.PortfolioMargin {
	req := types.DecimalFromInt(required)
	col := types.DecimalFromInt(onHand)
	return types.PortfolioMargin{
		ParticipantID:    participantID,
		TotalRequired:    req,
		CollateralOnHand: col,
		ExcessDeficit:    col.Sub(req), // Negative when onHand < required
	}
}

func surplusPortfolio(participantID string) types.PortfolioMargin {
	return types.PortfolioMargin{
		ParticipantID:    participantID,
		TotalRequired:    types.DecimalFromInt(50000),
		CollateralOnHand: types.DecimalFromInt(100000),
		ExcessDeficit:    types.DecimalFromInt(50000),
	}
}

func TestEvaluateIssuesCall(t *testing.T) {
	svc := NewService(&testIDGen{}, 1*time.Hour)
	pm := deficitPortfolio("P1", 100000, 50000)

	call, err := svc.Evaluate(pm)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if call == nil {
		t.Fatal("expected margin call to be issued")
	}
	if call.ParticipantID != "P1" {
		t.Errorf("expected participant P1, got %s", call.ParticipantID)
	}
	if call.Status != types.MarginCallIssued {
		t.Errorf("expected status ISSUED, got %s", call.Status.String())
	}
	// Deficit should be 50000
	expectedDeficit := types.DecimalFromInt(50000)
	if !call.Deficit.Equal(expectedDeficit) {
		t.Errorf("expected deficit %s, got %s", expectedDeficit.String(), call.Deficit.String())
	}
}

func TestEvaluateNoCallOnSurplus(t *testing.T) {
	svc := NewService(&testIDGen{}, 1*time.Hour)
	pm := surplusPortfolio("P1")

	call, err := svc.Evaluate(pm)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if call != nil {
		t.Error("no margin call should be issued for surplus")
	}
}

func TestEvaluateUpdatesExistingCall(t *testing.T) {
	svc := NewService(&testIDGen{}, 1*time.Hour)

	// First deficit
	call1, _ := svc.Evaluate(deficitPortfolio("P1", 100000, 50000))
	callID := call1.CallID

	// Second deficit (larger)
	call2, _ := svc.Evaluate(deficitPortfolio("P1", 150000, 50000))

	if call2.CallID != callID {
		t.Error("should update existing call, not create new one")
	}
	expectedDeficit := types.DecimalFromInt(100000)
	if !call2.Deficit.Equal(expectedDeficit) {
		t.Errorf("deficit should be updated to %s, got %s", expectedDeficit.String(), call2.Deficit.String())
	}
}

func TestEvaluateResolvesCall(t *testing.T) {
	svc := NewService(&testIDGen{}, 1*time.Hour)

	// Issue a call
	svc.Evaluate(deficitPortfolio("P1", 100000, 50000))

	// Now surplus resolves it
	call, _ := svc.Evaluate(surplusPortfolio("P1"))
	if call != nil {
		t.Error("call should be nil after resolution")
	}

	// Active should be gone
	_, ok := svc.GetActive("P1")
	if ok {
		t.Error("no active call should remain after resolution")
	}
}

func TestCheckDeadlines(t *testing.T) {
	svc := NewService(&testIDGen{}, 1*time.Hour)

	svc.Evaluate(deficitPortfolio("P1", 100000, 50000))
	svc.Evaluate(deficitPortfolio("P2", 200000, 80000))

	// Check before deadline
	breached := svc.CheckDeadlines(time.Now())
	if len(breached) != 0 {
		t.Errorf("no calls should be breached yet, got %d", len(breached))
	}

	// Check after deadline
	future := time.Now().Add(2 * time.Hour)
	breached = svc.CheckDeadlines(future)
	if len(breached) != 2 {
		t.Errorf("expected 2 breached calls, got %d", len(breached))
	}
	for _, b := range breached {
		if b.Status != types.MarginCallBreached {
			t.Errorf("expected BREACHED status, got %s", b.Status.String())
		}
	}
}

func TestGetActive(t *testing.T) {
	svc := NewService(&testIDGen{}, 1*time.Hour)

	_, ok := svc.GetActive("P1")
	if ok {
		t.Error("should not have active call for unknown participant")
	}

	svc.Evaluate(deficitPortfolio("P1", 100000, 50000))
	call, ok := svc.GetActive("P1")
	if !ok {
		t.Fatal("should have active call")
	}
	if call.ParticipantID != "P1" {
		t.Errorf("expected P1, got %s", call.ParticipantID)
	}
}

func TestGetAllActive(t *testing.T) {
	svc := NewService(&testIDGen{}, 1*time.Hour)

	svc.Evaluate(deficitPortfolio("P1", 100000, 50000))
	svc.Evaluate(deficitPortfolio("P2", 200000, 80000))

	active := svc.GetAllActive()
	if len(active) != 2 {
		t.Errorf("expected 2 active calls, got %d", len(active))
	}
}

func TestHandler(t *testing.T) {
	svc := NewService(&testIDGen{}, 1*time.Hour)

	var received *types.MarginCall
	svc.SetHandler(func(call types.MarginCall) {
		received = &call
	})

	svc.Evaluate(deficitPortfolio("P1", 100000, 50000))
	if received == nil {
		t.Fatal("handler should have been called")
	}
	if received.ParticipantID != "P1" {
		t.Errorf("handler received wrong participant: %s", received.ParticipantID)
	}
}

func TestStats(t *testing.T) {
	svc := NewService(&testIDGen{}, 1*time.Hour)

	svc.Evaluate(deficitPortfolio("P1", 100000, 50000))
	svc.Evaluate(deficitPortfolio("P2", 200000, 80000))

	// Resolve P1
	svc.Evaluate(surplusPortfolio("P1"))

	stats := svc.Stats()
	if stats.TotalIssued != 2 {
		t.Errorf("expected 2 total issued, got %d", stats.TotalIssued)
	}
	if stats.Active != 1 {
		t.Errorf("expected 1 active, got %d", stats.Active)
	}
	if stats.Satisfied != 1 {
		t.Errorf("expected 1 satisfied, got %d", stats.Satisfied)
	}
}
