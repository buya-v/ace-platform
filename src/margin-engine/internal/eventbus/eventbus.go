// Package eventbus is the margin-engine composition root for cross-service
// Kafka event propagation (R024). It consumes {tenant}.clearing.novated from
// the clearing engine, reconstructs the buyer/seller positions, recalculates
// portfolio margin, and publishes {tenant}.margin.call-issued whenever the
// margin engine issues a call.
//
// The bridge lives here (not in cmd/main.go) so it is unit testable against the
// in-process channel Producer/Consumer without a broker.
package eventbus

import (
	"context"
	"time"

	"github.com/garudax-platform/margin-engine/internal/engine"
	"github.com/garudax-platform/margin-engine/internal/kafka"
	"github.com/garudax-platform/margin-engine/internal/types"
)

// Enabled reports whether Kafka cross-service wiring is configured.
func Enabled() bool { return kafka.Enabled() }

// Runtime wires the margin engine to its upstream/downstream Kafka topics.
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

// PublishMarginCall publishes a margin.call-issued event for a call the engine
// has issued. The engine supports a single MarginCallHandler (last-write-wins),
// so main.go composes its persistence/logging handler with this method rather
// than the Runtime claiming the handler exclusively. The engine fires the
// handler outside its lock (R008), so publishing here is safe. Errors are
// swallowed: the call already exists in-engine and a transient publish failure
// must not undo it (the producer's own retry policy applies).
func (rt *Runtime) PublishMarginCall(call types.MarginCall) {
	_ = kafka.PublishMarginCallIssued(rt.producer, marginCallPayload(call), call.CallID)
}

// Start begins consuming. Blocks until ctx is cancelled.
func (rt *Runtime) Start(ctx context.Context) error { return rt.consumer.Start(ctx) }

// Close releases the consumer and producer.
func (rt *Runtime) Close() error {
	_ = rt.consumer.Close()
	return rt.producer.Close()
}

// onClearingNovated recalculates margin for both legs of a novated trade. A
// novation makes the buyer net-long and the seller net-short by Quantity at
// Price; the margin engine evaluates each leg and issues calls via the handler
// wired in newRuntime. Parse errors are non-retryable (return nil); engine
// errors are retryable (return err).
func (rt *Runtime) onClearingNovated(ctx context.Context, p kafka.ClearingNovatedPayload, correlationID string) error {
	price, err := types.ParseDecimal(p.Price)
	if err != nil {
		return nil // permanently bad record: drop rather than spin
	}
	qty := int64(p.Quantity)
	now := time.Now().UTC()

	legs := []struct {
		participant string
		net         int64
	}{
		{p.BuyerParticipantID, qty},
		{p.SellerParticipantID, -qty},
	}
	for _, leg := range legs {
		if leg.participant == "" {
			continue
		}
		pos := []types.Position{{
			ParticipantID: leg.participant,
			InstrumentID:  p.InstrumentID,
			NetQuantity:   leg.net,
			AvgEntryPrice: price,
			UpdatedAt:     now,
		}}
		if _, err := rt.eng.CalculateMargin(leg.participant, pos); err != nil {
			return err // retryable
		}
	}
	return nil
}

func marginCallPayload(call types.MarginCall) kafka.MarginCallIssuedPayload {
	return kafka.MarginCallIssuedPayload{
		MarginCallID:   call.CallID,
		ParticipantID:  call.ParticipantID,
		CallType:       "VARIATION",
		RequiredAmount: call.Required.String(),
		CurrentMargin:  call.OnHand.String(),
		Deficit:        call.Deficit.String(),
		Status:         call.Status.String(),
		Deadline:       call.Deadline.UTC().Format(time.RFC3339Nano),
		IssuedAt:       call.IssuedAt.UTC().Format(time.RFC3339Nano),
	}
}
