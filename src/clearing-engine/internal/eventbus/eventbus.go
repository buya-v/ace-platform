// Package eventbus is the clearing-engine composition root for cross-service
// Kafka event propagation (R024). It consumes {tenant}.trades.executed from the
// matching engine, runs each trade through novation via the clearing engine,
// and publishes {tenant}.clearing.novated for the margin and settlement engines
// to consume downstream.
//
// The decode -> ClearTrade -> publish bridge lives here (not in cmd/main.go) so
// it is unit testable against the in-process channel Producer/Consumer without a
// broker, and so the kafka package stays free of a dependency on the engine.
package eventbus

import (
	"context"
	"encoding/json"
	"log"

	"github.com/garudax-platform/clearing-engine/internal/engine"
	"github.com/garudax-platform/clearing-engine/internal/kafka"
	"github.com/garudax-platform/clearing-engine/internal/novation"
	"github.com/garudax-platform/clearing-engine/internal/types"
)

// Enabled reports whether Kafka cross-service wiring is configured.
func Enabled() bool { return kafka.Enabled() }

// Runtime wires the clearing engine to its upstream/downstream Kafka topics.
type Runtime struct {
	eng      *engine.Engine
	producer kafka.Producer
	consumer kafka.Consumer
}

// New builds a Runtime backed by env-configured Kafka adapters. Must only be
// called when Enabled() is true; otherwise the From-Env constructors fail fast
// per R007. The producer doubles as the consumer's DLQ sink.
func New(eng *engine.Engine) *Runtime {
	producer := kafka.NewProducerFromEnv()
	consumer := kafka.NewConsumerFromEnv(producer)
	return newRuntime(eng, producer, consumer)
}

// newRuntime is the test seam: callers inject explicit adapters.
func newRuntime(eng *engine.Engine, producer kafka.Producer, consumer kafka.Consumer) *Runtime {
	rt := &Runtime{eng: eng, producer: producer, consumer: consumer}
	consumer.Subscribe(kafka.TopicTradesExecuted, kafka.TradeExecutedHandler(rt.onTradeExecuted))
	return rt
}

// Start begins consuming. It blocks until ctx is cancelled (run in a goroutine).
func (rt *Runtime) Start(ctx context.Context) error {
	return rt.consumer.Start(ctx)
}

// Close releases the consumer and producer.
func (rt *Runtime) Close() error {
	_ = rt.consumer.Close()
	return rt.producer.Close()
}

// onTradeExecuted clears one trade and publishes the resulting novation. A
// decode/parse error returns an error so the consumer retries then DLQs; a
// successful clear publishes clearing.novated. The handler is idempotent at the
// consumer layer (processedIDs keyed on event ID).
func (rt *Runtime) onTradeExecuted(ctx context.Context, p kafka.TradeExecutedPayload, correlationID string) error {
	trade, err := tradeFromPayload(p)
	if err != nil {
		// Malformed price/quantity: not retryable. Log and drop (return nil so
		// the consumer does not spin retrying a permanently-bad record).
		log.Printf("[clearing-engine] dropping un-parseable trade %s: %v", p.TradeID, err)
		return nil
	}

	result, err := rt.eng.ClearTrade(trade)
	if err != nil {
		return err // retryable: transient engine/store failure
	}

	payload := novatedPayload(result)
	if err := kafka.PublishClearingNovated(rt.producer, payload, correlationID); err != nil {
		return err // retryable: publish failure
	}
	return nil
}

func tradeFromPayload(p kafka.TradeExecutedPayload) (types.Trade, error) {
	price, err := types.ParseDecimal(p.Price)
	if err != nil {
		return types.Trade{}, err
	}
	tradeValue, err := types.ParseDecimal(p.TradeValue)
	if err != nil {
		// TradeValue is derivable; fall back to price*quantity rather than fail.
		tradeValue = price.MulUint64(p.Quantity)
	}
	side := types.SideBuy
	if p.AggressorSide == "SELL" || p.AggressorSide == "sell" {
		side = types.SideSell
	}
	return types.Trade{
		TradeID:             p.TradeID,
		InstrumentID:        p.InstrumentID,
		BuyOrderID:          p.BuyOrderID,
		SellOrderID:         p.SellOrderID,
		BuyerParticipantID:  p.BuyerParticipantID,
		SellerParticipantID: p.SellerParticipantID,
		Price:               price,
		Quantity:            p.Quantity,
		TradeValue:          tradeValue,
		AggressorSide:       side,
		SequenceNumber:      p.SequenceNumber,
	}, nil
}

func novatedPayload(result *engine.ClearingResult) kafka.ClearingNovatedPayload {
	buyerPos, _ := json.Marshal(result.BuyerPosition)
	sellerPos, _ := json.Marshal(result.SellerPosition)
	return kafka.ClearingNovatedPayload{
		ObligationID:        result.Novation.BuyerObligation.ObligationID,
		TradeID:             result.Trade.TradeID,
		InstrumentID:        result.Trade.InstrumentID,
		BuyerParticipantID:  result.Trade.BuyerParticipantID,
		SellerParticipantID: result.Trade.SellerParticipantID,
		CCPID:               novation.CCP,
		Price:               result.Trade.Price.String(),
		Quantity:            result.Trade.Quantity,
		Status:              "NOVATED",
		BuyerPosition:       buyerPos,
		SellerPosition:      sellerPos,
	}
}
