package eventbus

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"github.com/garudax-platform/clearing-engine/internal/engine"
	"github.com/garudax-platform/clearing-engine/internal/kafka"
	"github.com/garudax-platform/clearing-engine/internal/store"
)

type seqIDGen struct{ n uint64 }

func (g *seqIDGen) NewID() string {
	return "obl-" + itoa(atomic.AddUint64(&g.n, 1))
}

func itoa(n uint64) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

func newTestEngine() *engine.Engine {
	return engine.NewEngine(&seqIDGen{}, store.NewInMemoryObligationStore())
}

func tradeEvent(t *testing.T, p kafka.TradeExecutedPayload) kafka.Record {
	t.Helper()
	evt, err := kafka.NewEvent(kafka.TopicTradesExecuted, "matching-engine", "corr-1", p)
	if err != nil {
		t.Fatalf("new event: %v", err)
	}
	data, err := json.Marshal(evt)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return kafka.Record{Topic: kafka.TopicTradesExecuted, Key: p.InstrumentID, Value: data}
}

func samplePayload() kafka.TradeExecutedPayload {
	return kafka.TradeExecutedPayload{
		TradeID:             "T-1",
		InstrumentID:        "WHT-HRW-2026M07-UB",
		BuyOrderID:          "B-1",
		SellOrderID:         "S-1",
		BuyerParticipantID:  "P-BUY",
		SellerParticipantID: "P-SELL",
		Price:               "250.50",
		Quantity:            10,
		TradeValue:          "2505.00",
		AggressorSide:       "BUY",
		TradeType:           "continuous",
		SequenceNumber:      1,
		ExecutedAt:          time.Now().UTC().Format(time.RFC3339Nano),
	}
}

// TestOnTradeExecuted_PublishesNovation drives the bridge directly: a decoded
// trade is cleared and a clearing.novated event is published.
func TestOnTradeExecuted_PublishesNovation(t *testing.T) {
	prod := kafka.NewChannelProducer(kafka.DefaultProducerConfig())
	prod.RegisterTopic(kafka.TopicClearingNovated, 10)
	cons := kafka.NewChannelConsumer(kafka.DefaultConsumerConfig("clearing-engine"), prod)
	rt := newRuntime(newTestEngine(), prod, cons)

	if err := rt.onTradeExecuted(context.Background(), samplePayload(), "corr-1"); err != nil {
		t.Fatalf("onTradeExecuted: %v", err)
	}

	recs := prod.Records(kafka.TopicClearingNovated)
	if len(recs) != 1 {
		t.Fatalf("expected 1 novated record, got %d", len(recs))
	}
	var evt kafka.Event
	if err := json.Unmarshal(recs[0].Value, &evt); err != nil {
		t.Fatalf("unmarshal event: %v", err)
	}
	var np kafka.ClearingNovatedPayload
	if err := json.Unmarshal(evt.Payload, &np); err != nil {
		t.Fatalf("unmarshal novated payload: %v", err)
	}
	if np.TradeID != "T-1" || np.InstrumentID != "WHT-HRW-2026M07-UB" {
		t.Errorf("novated ids wrong: %+v", np)
	}
	if np.Status != "NOVATED" {
		t.Errorf("status = %q, want NOVATED", np.Status)
	}
	if np.CCPID == "" {
		t.Error("CCPID should be set")
	}
	if np.BuyerParticipantID != "P-BUY" || np.SellerParticipantID != "P-SELL" {
		t.Errorf("participants wrong: %+v", np)
	}
}

// TestOnTradeExecuted_BadPriceDropped ensures an un-parseable price is dropped
// (returns nil) rather than retried forever.
func TestOnTradeExecuted_BadPriceDropped(t *testing.T) {
	prod := kafka.NewChannelProducer(kafka.DefaultProducerConfig())
	prod.RegisterTopic(kafka.TopicClearingNovated, 10)
	cons := kafka.NewChannelConsumer(kafka.DefaultConsumerConfig("clearing-engine"), prod)
	rt := newRuntime(newTestEngine(), prod, cons)

	bad := samplePayload()
	bad.Price = "not-a-number"
	if err := rt.onTradeExecuted(context.Background(), bad, "corr-1"); err != nil {
		t.Fatalf("bad price should be dropped (nil), got %v", err)
	}
	if recs := prod.Records(kafka.TopicClearingNovated); len(recs) != 0 {
		t.Errorf("expected no novation for bad trade, got %d", len(recs))
	}
}

// TestEndToEnd_ChannelBridge wires a channel producer's trades topic into the
// consumer's source and verifies the full consume->clear->publish path across
// the in-process bus.
func TestEndToEnd_ChannelBridge(t *testing.T) {
	novatedProd := kafka.NewChannelProducer(kafka.DefaultProducerConfig())
	novatedProd.RegisterTopic(kafka.TopicClearingNovated, 10)
	cons := kafka.NewChannelConsumer(kafka.DefaultConsumerConfig("clearing-engine"), novatedProd)

	src := make(chan kafka.Record, 1)
	cons.AddSource(kafka.TopicTradesExecuted, src)
	rt := newRuntime(newTestEngine(), novatedProd, cons)

	src <- tradeEvent(t, samplePayload())

	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = rt.Start(ctx) }()

	deadline := time.After(2 * time.Second)
	for {
		if recs := novatedProd.Records(kafka.TopicClearingNovated); len(recs) > 0 {
			cancel()
			return
		}
		select {
		case <-deadline:
			cancel()
			t.Fatal("timed out waiting for clearing.novated propagation")
		case <-time.After(10 * time.Millisecond):
		}
	}
}
