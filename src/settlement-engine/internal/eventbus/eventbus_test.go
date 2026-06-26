package eventbus

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"github.com/garudax-platform/settlement-engine/internal/engine"
	"github.com/garudax-platform/settlement-engine/internal/kafka"
	"github.com/garudax-platform/settlement-engine/internal/payment"
	"github.com/garudax-platform/settlement-engine/internal/valuation"
)

type seqIDGen struct{ n uint64 }

func (g *seqIDGen) NewID() string { return "si-" + itoa(atomic.AddUint64(&g.n, 1)) }

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
	return engine.NewEngine(valuation.NewStore(), &seqIDGen{}, payment.NewInMemoryGateway())
}

func novatedPayload() kafka.ClearingNovatedPayload {
	return kafka.ClearingNovatedPayload{
		ObligationID:        "obl-1",
		TradeID:             "T-1",
		InstrumentID:        "WHT-HRW-2026M07-UB",
		BuyerParticipantID:  "P-BUY",
		SellerParticipantID: "P-SELL",
		Price:               "250.00",
		Quantity:            10,
		Status:              "NOVATED",
	}
}

func TestOnClearingNovated_RunsCycleAndPublishesCompleted(t *testing.T) {
	prod := kafka.NewChannelProducer(kafka.DefaultProducerConfig())
	prod.RegisterTopic(kafka.TopicSettlementCompleted, 10)
	cons := kafka.NewChannelConsumer(kafka.DefaultConsumerConfig("settlement-engine"), prod)
	eng := newTestEngine()
	rt := newRuntime(eng, prod, cons)
	// Compose like main.go: the single cycle handler publishes.
	eng.SetCycleHandler(rt.PublishCycle)

	if err := rt.onClearingNovated(context.Background(), novatedPayload(), "corr-1"); err != nil {
		t.Fatalf("onClearingNovated: %v", err)
	}

	recs := prod.Records(kafka.TopicSettlementCompleted)
	if len(recs) != 1 {
		t.Fatalf("expected 1 settlement.completed event, got %d", len(recs))
	}
	var evt kafka.Event
	if err := json.Unmarshal(recs[0].Value, &evt); err != nil {
		t.Fatalf("unmarshal event: %v", err)
	}
	var sc kafka.SettlementCompletedPayload
	if err := json.Unmarshal(evt.Payload, &sc); err != nil {
		t.Fatalf("unmarshal settlement payload: %v", err)
	}
	if sc.CycleID != "cycle-T-1" {
		t.Errorf("cycle id = %q, want cycle-T-1 (deterministic, idempotent)", sc.CycleID)
	}
	if sc.Status == "" {
		t.Errorf("status should be set: %+v", sc)
	}
	if len(sc.SettlementPrices) == 0 {
		t.Errorf("expected settlement prices populated from P&L records")
	}
}

func TestOnClearingNovated_MissingParticipantDropped(t *testing.T) {
	prod := kafka.NewChannelProducer(kafka.DefaultProducerConfig())
	prod.RegisterTopic(kafka.TopicSettlementCompleted, 10)
	cons := kafka.NewChannelConsumer(kafka.DefaultConsumerConfig("settlement-engine"), prod)
	rt := newRuntime(newTestEngine(), prod, cons)

	p := novatedPayload()
	p.SellerParticipantID = ""
	if err := rt.onClearingNovated(context.Background(), p, "corr-1"); err != nil {
		t.Fatalf("missing participant should be dropped (nil), got %v", err)
	}
	if recs := prod.Records(kafka.TopicSettlementCompleted); len(recs) != 0 {
		t.Errorf("expected no settlement for incomplete novation, got %d", len(recs))
	}
}

func TestEndToEnd_ChannelBridge(t *testing.T) {
	doneProd := kafka.NewChannelProducer(kafka.DefaultProducerConfig())
	doneProd.RegisterTopic(kafka.TopicSettlementCompleted, 10)
	cons := kafka.NewChannelConsumer(kafka.DefaultConsumerConfig("settlement-engine"), doneProd)
	src := make(chan kafka.Record, 1)
	cons.AddSource(kafka.TopicClearingNovated, src)
	eng := newTestEngine()
	rt := newRuntime(eng, doneProd, cons)
	eng.SetCycleHandler(rt.PublishCycle)

	evt, err := kafka.NewEvent(kafka.TopicClearingNovated, "clearing-engine", "corr-1", novatedPayload())
	if err != nil {
		t.Fatalf("new event: %v", err)
	}
	data, _ := json.Marshal(evt)
	src <- kafka.Record{Topic: kafka.TopicClearingNovated, Key: "WHT-HRW-2026M07-UB", Value: data}

	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = rt.Start(ctx) }()
	deadline := time.After(2 * time.Second)
	for {
		if recs := doneProd.Records(kafka.TopicSettlementCompleted); len(recs) > 0 {
			cancel()
			return
		}
		select {
		case <-deadline:
			cancel()
			t.Fatal("timed out waiting for settlement.completed propagation")
		case <-time.After(10 * time.Millisecond):
		}
	}
}
