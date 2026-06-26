package eventbus

import (
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/garudax-platform/matching-engine/internal/kafka"
	"github.com/garudax-platform/matching-engine/internal/types"
)

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func sampleTrade() types.Trade {
	return types.Trade{
		TradeID:             "T-1",
		InstrumentID:        "WHT-HRW-2026M07-UB",
		BuyOrderID:          "B-1",
		SellOrderID:         "S-1",
		BuyerParticipantID:  "P-BUY",
		SellerParticipantID: "P-SELL",
		Price:               types.NewDecimal(250, 5000), // 250.5
		Quantity:            10,
		TradeValue:          types.NewDecimal(2505, 0),
		AggressorSide:       types.SideBuy,
		TradeType:           types.TradeTypeContinuous,
		SequenceNumber:      42,
		ExecutedAt:          time.Date(2026, 6, 26, 10, 0, 0, 0, time.UTC),
	}
}

func TestPublishTrade_EmitsTradesExecuted(t *testing.T) {
	prod := kafka.NewChannelProducer(kafka.DefaultProducerConfig())
	prod.RegisterTopic(kafka.TopicTradesExecuted, 10)
	pub := NewPublisherWith(prod, quietLogger())

	pub.PublishTrade(sampleTrade())

	recs := prod.Records(kafka.TopicTradesExecuted)
	if len(recs) != 1 {
		t.Fatalf("expected 1 record on %s, got %d", kafka.TopicTradesExecuted, len(recs))
	}

	var evt kafka.Event
	if err := json.Unmarshal(recs[0].Value, &evt); err != nil {
		t.Fatalf("unmarshal event: %v", err)
	}
	if evt.Type != kafka.TopicTradesExecuted {
		t.Errorf("event type = %q, want %q", evt.Type, kafka.TopicTradesExecuted)
	}
	if recs[0].Key != "WHT-HRW-2026M07-UB" {
		t.Errorf("partition key = %q, want instrument id", recs[0].Key)
	}

	var payload kafka.TradeExecutedPayload
	if err := json.Unmarshal(evt.Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.TradeID != "T-1" || payload.InstrumentID != "WHT-HRW-2026M07-UB" {
		t.Errorf("payload ids wrong: %+v", payload)
	}
	if payload.Price != "250.5" {
		t.Errorf("payload price = %q, want 250.5", payload.Price)
	}
	if payload.Quantity != 10 {
		t.Errorf("payload quantity = %d, want 10", payload.Quantity)
	}
	if payload.AggressorSide != "BUY" {
		t.Errorf("aggressor side = %q, want BUY", payload.AggressorSide)
	}
	if payload.TradeType != "continuous" {
		t.Errorf("trade type = %q, want continuous", payload.TradeType)
	}
}

func TestPublishTrade_PublishErrorDoesNotPanic(t *testing.T) {
	// Producer with the topic NOT registered + closed: Publish returns an error,
	// which PublishTrade must log and swallow (never crash the matching path).
	prod := kafka.NewChannelProducer(kafka.DefaultProducerConfig())
	_ = prod.Close()
	pub := NewPublisherWith(prod, quietLogger())
	pub.PublishTrade(sampleTrade()) // must not panic
}

func TestTradeTypeString(t *testing.T) {
	cases := map[types.TradeType]string{
		types.TradeTypeContinuous:  "continuous",
		types.TradeTypeAuction:     "auction",
		types.TradeTypeUnspecified: "unspecified",
	}
	for in, want := range cases {
		if got := tradeTypeString(in); got != want {
			t.Errorf("tradeTypeString(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestEnabled_FalseWithoutBrokers(t *testing.T) {
	t.Setenv("KAFKA_BROKERS", "")
	if Enabled() {
		t.Error("Enabled() should be false when KAFKA_BROKERS is empty")
	}
	t.Setenv("KAFKA_BROKERS", "localhost:9092")
	if !Enabled() {
		t.Error("Enabled() should be true when KAFKA_BROKERS is set")
	}
}
