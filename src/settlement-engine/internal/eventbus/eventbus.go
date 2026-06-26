// Package eventbus is the settlement-engine composition root for cross-service
// Kafka event propagation (R024). It consumes {tenant}.clearing.novated from the
// clearing engine, runs a mark-to-market settlement cycle for the novated
// positions, and publishes {tenant}.settlement.completed — the terminal event of
// the matching -> clearing -> margin/settlement chain.
//
// The bridge lives here (not in cmd/main.go) so it is unit testable against the
// in-process channel Producer/Consumer without a broker.
package eventbus

import (
	"context"
	"time"

	"github.com/garudax-platform/settlement-engine/internal/engine"
	"github.com/garudax-platform/settlement-engine/internal/kafka"
	"github.com/garudax-platform/settlement-engine/internal/types"
)

// Enabled reports whether Kafka cross-service wiring is configured.
func Enabled() bool { return kafka.Enabled() }

// Runtime wires the settlement engine to its upstream/downstream Kafka topics.
type Runtime struct {
	eng      *engine.Engine
	producer kafka.Producer
	consumer kafka.Consumer
}

// New builds a Runtime backed by env-configured Kafka adapters. Must only be
// called when Enabled() is true.
func New(eng *engine.Engine) *Runtime {
	producer := kafka.NewProducerFromEnv()
	consumer := kafka.NewConsumerFromEnv(producer)
	return newRuntime(eng, producer, consumer)
}

// newRuntime is the test seam: callers inject explicit adapters.
func newRuntime(eng *engine.Engine, producer kafka.Producer, consumer kafka.Consumer) *Runtime {
	rt := &Runtime{eng: eng, producer: producer, consumer: consumer}
	consumer.Subscribe(kafka.TopicClearingNovated, kafka.ClearingNovatedHandler(rt.onClearingNovated))
	return rt
}

// Start begins consuming. Blocks until ctx is cancelled.
func (rt *Runtime) Start(ctx context.Context) error { return rt.consumer.Start(ctx) }

// Close releases the consumer and producer.
func (rt *Runtime) Close() error {
	_ = rt.consumer.Close()
	return rt.producer.Close()
}

// PublishCycle publishes a settlement.completed event for a completed cycle. The
// settlement engine supports a single CycleHandler (last-write-wins), so main.go
// composes its logging handler with this method. The engine fires the handler
// outside its lock (R008). Errors are swallowed: the cycle already ran; a
// transient publish failure must not undo it (producer retry applies).
func (rt *Runtime) PublishCycle(cycle types.SettlementCycle) {
	_ = kafka.PublishSettlementCompleted(rt.producer, cyclePayload(cycle), cycle.CycleID)
}

// onClearingNovated runs a settlement cycle for one novated trade. The novated
// positions (buyer net-long, seller net-short at Price) are marked at the trade
// price (a same-day novation marks to its own fill), and a deterministic cycle
// ID keyed on TradeID keeps reprocessing idempotent. Parse errors are
// non-retryable (return nil); engine errors are retryable (return err). The
// settlement.completed publication happens via the CycleHandler wired in main.go.
func (rt *Runtime) onClearingNovated(ctx context.Context, p kafka.ClearingNovatedPayload, correlationID string) error {
	price, err := types.ParseDecimal(p.Price)
	if err != nil {
		return nil
	}
	if p.BuyerParticipantID == "" || p.SellerParticipantID == "" {
		return nil
	}
	settleDate := time.Now().UTC().Truncate(24 * time.Hour)
	// A settlement price is required for the P&L calculation; absent any external
	// mark, settle the novation at its own fill price (zero same-day variation).
	rt.eng.SetSettlementPrice(p.InstrumentID, settleDate, price)

	qty := int64(p.Quantity)
	now := time.Now().UTC()
	positions := []types.Position{
		{ParticipantID: p.BuyerParticipantID, InstrumentID: p.InstrumentID, NetQuantity: qty, AvgEntryPrice: price, UpdatedAt: now},
		{ParticipantID: p.SellerParticipantID, InstrumentID: p.InstrumentID, NetQuantity: -qty, AvgEntryPrice: price, UpdatedAt: now},
	}
	cycleID := "cycle-" + p.TradeID
	if _, err := rt.eng.RunSettlementCycle(cycleID, settleDate, positions); err != nil {
		return err
	}
	return nil
}

func cyclePayload(cycle types.SettlementCycle) kafka.SettlementCompletedPayload {
	prices := make([]kafka.SettlementPriceEntry, 0, len(cycle.PnLRecords))
	seen := map[string]bool{}
	for _, rec := range cycle.PnLRecords {
		if seen[rec.InstrumentID] {
			continue
		}
		seen[rec.InstrumentID] = true
		prices = append(prices, kafka.SettlementPriceEntry{
			InstrumentID:    rec.InstrumentID,
			SettlementPrice: rec.CurrentPrice.String(),
			PreviousPrice:   rec.PreviousPrice.String(),
		})
	}
	return kafka.SettlementCompletedPayload{
		CycleID:           cycle.CycleID,
		SettleDate:        cycle.SettleDate.UTC().Format("2006-01-02"),
		Status:            cycle.Status.String(),
		SettlementPrices:  prices,
		TotalPayIn:        cycle.TotalPayIn.String(),
		TotalPayOut:       cycle.TotalPayOut.String(),
		InstructionsCount: len(cycle.Instructions),
		StartedAt:         cycle.StartedAt.UTC().Format(time.RFC3339Nano),
		CompletedAt:       cycle.CompletedAt.UTC().Format(time.RFC3339Nano),
	}
}
