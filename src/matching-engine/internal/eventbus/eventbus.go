// Package eventbus is the matching-engine composition root for cross-service
// Kafka event propagation (R024). It bridges the in-process matching engine to
// the platform Kafka topics: every matched trade is published to
// {tenant}.trades.executed, which the clearing engine consumes downstream.
//
// The conversion + publish logic lives here (not in cmd/main.go) so it is unit
// testable against the in-process channel Producer without a broker, and so the
// kafka package stays free of a dependency on internal/types.
package eventbus

import (
	"log/slog"
	"time"

	"github.com/garudax-platform/matching-engine/internal/kafka"
	"github.com/garudax-platform/matching-engine/internal/types"
)

// Enabled reports whether Kafka cross-service wiring is configured for this
// process (KAFKA_BROKERS set). When false, the caller should skip wiring.
func Enabled() bool { return kafka.Enabled() }

// Publisher publishes matched trades to the trades.executed topic.
type Publisher struct {
	producer kafka.Producer
	logger   *slog.Logger
}

// NewPublisher constructs a Publisher backed by the env-configured Kafka
// producer. It must only be called when Enabled() is true; otherwise
// kafka.NewProducerFromEnv fails fast per R007.
func NewPublisher(logger *slog.Logger) *Publisher {
	return &Publisher{producer: kafka.NewProducerFromEnv(), logger: logger}
}

// NewPublisherWith builds a Publisher around an explicit producer (test seam).
func NewPublisherWith(producer kafka.Producer, logger *slog.Logger) *Publisher {
	return &Publisher{producer: producer, logger: logger}
}

// PublishTrade converts a matched trade into the canonical trades.executed
// envelope and publishes it, keyed by instrument_id. Publish errors are logged
// and swallowed: a Kafka outage must never block or crash the matching hot path
// (the real adapter already retries per ProducerConfig; persistent failure is a
// telemetry/DLQ concern, not a matching concern).
func (p *Publisher) PublishTrade(trade types.Trade) {
	payload := kafka.TradeExecutedPayload{
		TradeID:             trade.TradeID,
		InstrumentID:        trade.InstrumentID,
		BuyOrderID:          trade.BuyOrderID,
		SellOrderID:         trade.SellOrderID,
		BuyerParticipantID:  trade.BuyerParticipantID,
		SellerParticipantID: trade.SellerParticipantID,
		Price:               trade.Price.String(),
		Quantity:            trade.Quantity,
		TradeValue:          trade.TradeValue.String(),
		AggressorSide:       trade.AggressorSide.String(),
		TradeType:           tradeTypeString(trade.TradeType),
		SequenceNumber:      trade.SequenceNumber,
		ExecutedAt:          trade.ExecutedAt.UTC().Format(time.RFC3339Nano),
	}
	if err := kafka.PublishTradeExecuted(p.producer, payload, trade.TradeID); err != nil {
		p.logger.Error("kafka_publish_trade_failed",
			slog.String("trade_id", trade.TradeID),
			slog.String("instrument_id", trade.InstrumentID),
			slog.String("error", err.Error()),
		)
	}
}

// Close releases the underlying producer.
func (p *Publisher) Close() error {
	if p.producer == nil {
		return nil
	}
	return p.producer.Close()
}

func tradeTypeString(t types.TradeType) string {
	switch t {
	case types.TradeTypeContinuous:
		return "continuous"
	case types.TradeTypeAuction:
		return "auction"
	default:
		return "unspecified"
	}
}
