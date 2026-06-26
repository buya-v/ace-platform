package eventbus

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"github.com/garudax-platform/margin-engine/internal/engine"
	"github.com/garudax-platform/margin-engine/internal/kafka"
	"github.com/garudax-platform/margin-engine/internal/params"
	"github.com/garudax-platform/margin-engine/internal/types"
)

type seqIDGen struct{ n uint64 }

func (g *seqIDGen) NewID() string { return "mc-" + itoa(atomic.AddUint64(&g.n, 1)) }

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

// zeroCollateral always reports no collateral, so any margin requirement on an
// open position triggers a margin call.
type zeroCollateral struct{}

func (zeroCollateral) GetCollateral(string) types.Decimal { return types.DecimalZero() }

func newTestEngine() *engine.Engine {
	ps := params.NewStore()
	ps.Set(params.InstrumentParams{
		InstrumentID:   "WHT-HRW-2026M07-UB",
		PriceScanRange: types.NewDecimal(3, 0),
		VolScanRange:   types.NewDecimal(0, 5000),
		SpotPrice:      types.NewDecimal(250, 0),
		ContractSize:   1,
	})
	return engine.NewEngine(ps, &seqIDGen{}, zeroCollateral{}, time.Hour)
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

func TestOnClearingNovated_IssuesAndPublishesMarginCall(t *testing.T) {
	prod := kafka.NewChannelProducer(kafka.DefaultProducerConfig())
	prod.RegisterTopic(kafka.TopicMarginCallIssued, 10)
	cons := kafka.NewChannelConsumer(kafka.DefaultConsumerConfig("margin-engine"), prod)
	eng := newTestEngine()
	rt := newRuntime(eng, prod, cons)
	// Compose like main.go: the engine's single margin-call handler publishes.
	eng.SetMarginCallHandler(rt.PublishMarginCall)

	if err := rt.onClearingNovated(context.Background(), novatedPayload(), "corr-1"); err != nil {
		t.Fatalf("onClearingNovated: %v", err)
	}

	recs := prod.Records(kafka.TopicMarginCallIssued)
	if len(recs) == 0 {
		t.Fatal("expected at least one margin.call-issued event")
	}
	var evt kafka.Event
	if err := json.Unmarshal(recs[0].Value, &evt); err != nil {
		t.Fatalf("unmarshal event: %v", err)
	}
	var mc kafka.MarginCallIssuedPayload
	if err := json.Unmarshal(evt.Payload, &mc); err != nil {
		t.Fatalf("unmarshal margin call payload: %v", err)
	}
	if mc.ParticipantID == "" || mc.MarginCallID == "" {
		t.Errorf("margin call payload incomplete: %+v", mc)
	}
	if mc.RequiredAmount == "" || mc.Deficit == "" {
		t.Errorf("margin call amounts missing: %+v", mc)
	}
}

func TestOnClearingNovated_BadPriceDropped(t *testing.T) {
	prod := kafka.NewChannelProducer(kafka.DefaultProducerConfig())
	prod.RegisterTopic(kafka.TopicMarginCallIssued, 10)
	cons := kafka.NewChannelConsumer(kafka.DefaultConsumerConfig("margin-engine"), prod)
	rt := newRuntime(newTestEngine(), prod, cons)

	bad := novatedPayload()
	bad.Price = "xyz"
	if err := rt.onClearingNovated(context.Background(), bad, "corr-1"); err != nil {
		t.Fatalf("bad price should be dropped (nil), got %v", err)
	}
}

func TestEndToEnd_ChannelBridge(t *testing.T) {
	callProd := kafka.NewChannelProducer(kafka.DefaultProducerConfig())
	callProd.RegisterTopic(kafka.TopicMarginCallIssued, 10)
	cons := kafka.NewChannelConsumer(kafka.DefaultConsumerConfig("margin-engine"), callProd)
	src := make(chan kafka.Record, 1)
	cons.AddSource(kafka.TopicClearingNovated, src)
	eng := newTestEngine()
	rt := newRuntime(eng, callProd, cons)
	eng.SetMarginCallHandler(rt.PublishMarginCall)

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
		if recs := callProd.Records(kafka.TopicMarginCallIssued); len(recs) > 0 {
			cancel()
			return
		}
		select {
		case <-deadline:
			cancel()
			t.Fatal("timed out waiting for margin.call-issued propagation")
		case <-time.After(10 * time.Millisecond):
		}
	}
}
