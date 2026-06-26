package engine

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/garudax-platform/settlement-engine/internal/types"
)

// TestGetInstrumentConfigConcurrentWithRegister runs RegisterInstrument (which
// writes e.instruments) concurrently with getInstrumentConfig (which reads it).
// Run with -race to confirm the read is guarded — previously getInstrumentConfig
// read the map with no lock, racing with RegisterInstrument's write.
func TestGetInstrumentConfigConcurrentWithRegister(t *testing.T) {
	eng, _, _ := setupEngine(t)

	const workers = 16
	var wg sync.WaitGroup
	wg.Add(workers * 2)

	for w := 0; w < workers; w++ {
		go func(w int) {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				eng.RegisterInstrument(types.InstrumentConfig{
					InstrumentID: fmt.Sprintf("INSTR-%d-%d", w, i),
					Type:         types.InstrumentCashSettled,
				})
			}
		}(w)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				_ = eng.getInstrumentConfig(fmt.Sprintf("INSTR-%d-%d", w, i))
			}
		}(w)
	}

	wg.Wait()
}

// TestRunSettlementCycleHandlerOutsideLock verifies the cycle handler runs
// after e.mu is released. The handler re-enters the engine via GetAllCycles
// (which takes the read lock); if the handler ran inside the write-locked
// section this would deadlock.
func TestRunSettlementCycleHandlerOutsideLock(t *testing.T) {
	eng, priceStore, _ := setupEngine(t)

	day1 := time.Date(2026, 3, 26, 0, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 3, 27, 0, 0, 0, 0, time.UTC)
	priceStore.SetSettlementPrice("WHEAT-MAY26", day1, types.NewDecimal(1500, 0))
	priceStore.SetSettlementPrice("WHEAT-MAY26", day2, types.NewDecimal(1520, 0))

	eng.SetCycleHandler(func(_ types.SettlementCycle) {
		// Re-enter the engine — must not deadlock now that the handler runs
		// outside the lock.
		_ = eng.GetAllCycles()
	})

	positions := []types.Position{
		{ParticipantID: "P1", InstrumentID: "WHEAT-MAY26", NetQuantity: 10},
		{ParticipantID: "P2", InstrumentID: "WHEAT-MAY26", NetQuantity: -10},
	}

	done := make(chan struct{})
	go func() {
		eng.RunSettlementCycle("cycle-1", day2, positions)
		close(done)
	}()

	select {
	case <-done:
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("RunSettlementCycle deadlocked — handler is still being invoked inside the lock")
	}
}

// TestRunSettlementCycleConcurrentRace runs settlement cycles, instrument
// registration, handler mutation, and cycle reads concurrently. Run with -race.
func TestRunSettlementCycleConcurrentRace(t *testing.T) {
	eng, priceStore, _ := setupEngine(t)

	day1 := time.Date(2026, 3, 26, 0, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 3, 27, 0, 0, 0, 0, time.UTC)
	priceStore.SetSettlementPrice("WHEAT-MAY26", day1, types.NewDecimal(1500, 0))
	priceStore.SetSettlementPrice("WHEAT-MAY26", day2, types.NewDecimal(1520, 0))

	positions := []types.Position{
		{ParticipantID: "P1", InstrumentID: "WHEAT-MAY26", NetQuantity: 10},
		{ParticipantID: "P2", InstrumentID: "WHEAT-MAY26", NetQuantity: -10},
	}

	const workers = 8
	var wg sync.WaitGroup
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		go func(w int) {
			defer wg.Done()
			for i := 0; i < 10; i++ {
				eng.RunSettlementCycle(fmt.Sprintf("cycle-%d-%d", w, i), day2, positions)
			}
		}(w)
	}

	wg.Add(3)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			eng.RegisterInstrument(types.InstrumentConfig{InstrumentID: fmt.Sprintf("X-%d", i), Type: types.InstrumentCashSettled})
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			eng.SetCycleHandler(func(_ types.SettlementCycle) {})
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			_ = eng.GetAllCycles()
		}
	}()

	wg.Wait()
}
