package kafka

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/garudax-platform/shared/internal/tenant"
)

const (
	ace = tenant.TenantID("ace-commodities")
	mse = tenant.TenantID("mse-equities")
)

// ── Topic construction & parsing ──────────────────────────────────────────

func TestBuildTopic(t *testing.T) {
	tests := []struct {
		name    string
		tenant  tenant.TenantID
		domain  string
		event   string
		want    string
		wantErr error
	}{
		{"simple", ace, "trades", "executed", "ace-commodities.trades.executed", nil},
		{"hyphenated domain", mse, "market-data", "tick", "mse-equities.market-data.tick", nil},
		{"hyphenated event", ace, "margin", "call-issued", "ace-commodities.margin.call-issued", nil},
		{"empty tenant", "", "trades", "executed", "", ErrInvalidSegment},
		{"empty domain", ace, "", "executed", "", ErrInvalidSegment},
		{"empty event", ace, "trades", "", "", ErrInvalidSegment},
		{"uppercase tenant", "ACE", "trades", "executed", "", ErrInvalidSegment},
		{"dot in domain", ace, "trades.x", "executed", "", ErrInvalidSegment},
		{"underscore", ace, "trades_x", "executed", "", ErrInvalidSegment},
		{"leading hyphen", ace, "-trades", "executed", "", ErrInvalidSegment},
		{"trailing hyphen", ace, "trades-", "executed", "", ErrInvalidSegment},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := BuildTopic(tc.tenant, tc.domain, tc.event)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("expected error %v, got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestParseTopic(t *testing.T) {
	tests := []struct {
		name    string
		topic   string
		want    Topic
		wantErr error
	}{
		{"simple", "ace-commodities.trades.executed", Topic{ace, "trades", "executed"}, nil},
		{"hyphenated domain", "mse-equities.market-data.tick", Topic{mse, "market-data", "tick"}, nil},
		{"too few segments", "ace-commodities.trades", Topic{}, ErrInvalidTopic},
		{"single segment", "orders", Topic{}, ErrInvalidTopic},
		{"empty", "", Topic{}, ErrInvalidTopic},
		{"bad tenant", "ACE.trades.executed", Topic{}, ErrInvalidSegment},
		{"bad event", "ace-commodities.trades.EXECUTED", Topic{}, ErrInvalidSegment},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseTopic(tc.topic)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("expected error %v, got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestTopicRoundTrip(t *testing.T) {
	topic, err := BuildTopic(mse, "clearing", "novated")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseTopic(topic)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.String() != topic {
		t.Fatalf("round trip mismatch: %q != %q", parsed.String(), topic)
	}
}

func TestTopicBelongsTo(t *testing.T) {
	if !TopicBelongsTo("ace-commodities.trades.executed", ace) {
		t.Error("expected topic to belong to ace-commodities")
	}
	if TopicBelongsTo("ace-commodities.trades.executed", mse) {
		t.Error("expected topic NOT to belong to mse-equities")
	}
	if TopicBelongsTo("garbage", ace) {
		t.Error("malformed topic must not belong to any tenant")
	}
}

// ── Test doubles ──────────────────────────────────────────────────────────

type capturedPublish struct {
	topic string
	key   string
	value []byte
	ctxID tenant.TenantID
}

type fakePublisher struct {
	mu       sync.Mutex
	captured []capturedPublish
	err      error
}

func (f *fakePublisher) Publish(ctx context.Context, topic, key string, value []byte) error {
	if f.err != nil {
		return f.err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	id, _ := tenant.TenantFromContext(ctx)
	f.captured = append(f.captured, capturedPublish{topic, key, value, id})
	return nil
}

func (f *fakePublisher) last() capturedPublish {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.captured[len(f.captured)-1]
}

type fakeSubscriber struct {
	handlers map[string]RawHandler
}

func newFakeSubscriber() *fakeSubscriber {
	return &fakeSubscriber{handlers: make(map[string]RawHandler)}
}

func (f *fakeSubscriber) Subscribe(topic string, handler RawHandler) error {
	f.handlers[topic] = handler
	return nil
}

// deliver simulates the transport routing a record to whatever handler is
// registered for topic (or, for the leakage tests, any handler we pick).
func (f *fakeSubscriber) deliver(handlerTopic string, d Delivery) error {
	h, ok := f.handlers[handlerTopic]
	if !ok {
		return errors.New("no handler")
	}
	return h(context.Background(), d)
}

// ── Producer ──────────────────────────────────────────────────────────────

func TestProducerContextDerived(t *testing.T) {
	pub := &fakePublisher{}
	p := NewTenantProducer(pub)

	ctx := tenant.WithTenant(context.Background(), ace)
	if err := p.Publish(ctx, "trades", "executed", "INST-1", []byte(`{}`)); err != nil {
		t.Fatal(err)
	}

	got := pub.last()
	if got.topic != "ace-commodities.trades.executed" {
		t.Fatalf("wrong topic: %q", got.topic)
	}
	if got.ctxID != ace {
		t.Fatalf("tenant not propagated to transport ctx: %q", got.ctxID)
	}
}

func TestProducerNoTenant(t *testing.T) {
	p := NewTenantProducer(&fakePublisher{})
	err := p.Publish(context.Background(), "trades", "executed", "k", nil)
	if !errors.Is(err, ErrNoTenant) {
		t.Fatalf("expected ErrNoTenant, got %v", err)
	}
}

func TestBoundProducerUsesBinding(t *testing.T) {
	pub := &fakePublisher{}
	p, err := NewBoundProducer(pub, mse)
	if err != nil {
		t.Fatal(err)
	}
	// No tenant in context — binding supplies it.
	if err := p.Publish(context.Background(), "clearing", "novated", "k", nil); err != nil {
		t.Fatal(err)
	}
	if got := pub.last().topic; got != "mse-equities.clearing.novated" {
		t.Fatalf("wrong topic: %q", got)
	}
}

func TestBoundProducerRejectsCrossTenant(t *testing.T) {
	pub := &fakePublisher{}
	p, err := NewBoundProducer(pub, ace)
	if err != nil {
		t.Fatal(err)
	}
	// Context says mse but producer is bound to ace -> leak attempt.
	ctx := tenant.WithTenant(context.Background(), mse)
	err = p.Publish(ctx, "trades", "executed", "k", nil)
	if !errors.Is(err, ErrCrossTenant) {
		t.Fatalf("expected ErrCrossTenant, got %v", err)
	}
	if len(pub.captured) != 0 {
		t.Fatal("nothing should have been published on a cross-tenant rejection")
	}
}

func TestBoundProducerAllowsMatchingContext(t *testing.T) {
	pub := &fakePublisher{}
	p, _ := NewBoundProducer(pub, ace)
	ctx := tenant.WithTenant(context.Background(), ace)
	if err := p.Publish(ctx, "trades", "executed", "k", nil); err != nil {
		t.Fatalf("matching context should be allowed: %v", err)
	}
}

func TestNewBoundProducerValidatesTenant(t *testing.T) {
	if _, err := NewBoundProducer(&fakePublisher{}, "BAD_ID"); !errors.Is(err, ErrInvalidSegment) {
		t.Fatalf("expected ErrInvalidSegment, got %v", err)
	}
}

func TestProducerRejectsBadDomain(t *testing.T) {
	p := NewTenantProducer(&fakePublisher{})
	ctx := tenant.WithTenant(context.Background(), ace)
	if err := p.Publish(ctx, "bad domain", "executed", "k", nil); !errors.Is(err, ErrInvalidSegment) {
		t.Fatalf("expected ErrInvalidSegment, got %v", err)
	}
}

// ── Consumer ──────────────────────────────────────────────────────────────

func TestConsumerSubscribeAndDeliver(t *testing.T) {
	sub := newFakeSubscriber()
	c, err := NewTenantConsumer(sub, ace)
	if err != nil {
		t.Fatal(err)
	}

	var gotKey string
	var gotTenant tenant.TenantID
	err = c.Subscribe("trades", "executed", func(ctx context.Context, key string, value []byte) error {
		gotKey = key
		gotTenant = tenant.MustTenant(ctx) // must not panic: tenant injected
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	topic := "ace-commodities.trades.executed"
	if _, ok := sub.handlers[topic]; !ok {
		t.Fatalf("handler not registered for %q", topic)
	}
	if err := sub.deliver(topic, Delivery{Topic: topic, Key: "INST-1", Value: []byte(`{}`)}); err != nil {
		t.Fatalf("deliver failed: %v", err)
	}
	if gotKey != "INST-1" {
		t.Fatalf("handler got wrong key: %q", gotKey)
	}
	if gotTenant != ace {
		t.Fatalf("handler got wrong tenant: %q", gotTenant)
	}
}

func TestConsumerRejectsCrossTenantDelivery(t *testing.T) {
	sub := newFakeSubscriber()
	c, _ := NewTenantConsumer(sub, ace)

	called := false
	c.Subscribe("trades", "executed", func(ctx context.Context, key string, value []byte) error {
		called = true
		return nil
	})

	// The transport misroutes an mse record to the ace handler.
	aceTopic := "ace-commodities.trades.executed"
	err := sub.deliver(aceTopic, Delivery{
		Topic: "mse-equities.trades.executed", // wrong tenant!
		Key:   "INST-1",
		Value: []byte(`{}`),
	})
	if !errors.Is(err, ErrCrossTenant) {
		t.Fatalf("expected ErrCrossTenant, got %v", err)
	}
	if called {
		t.Fatal("handler must NOT run for a cross-tenant record")
	}
}

func TestConsumerRejectsMalformedTopic(t *testing.T) {
	sub := newFakeSubscriber()
	c, _ := NewTenantConsumer(sub, ace)
	aceTopic := "ace-commodities.trades.executed"
	c.Subscribe("trades", "executed", func(ctx context.Context, key string, value []byte) error {
		t.Fatal("handler must not run for malformed topic")
		return nil
	})
	err := sub.deliver(aceTopic, Delivery{Topic: "garbage", Key: "k", Value: nil})
	if !errors.Is(err, ErrCrossTenant) {
		t.Fatalf("expected ErrCrossTenant, got %v", err)
	}
}

func TestNewTenantConsumerValidatesTenant(t *testing.T) {
	if _, err := NewTenantConsumer(newFakeSubscriber(), "Bad.Id"); !errors.Is(err, ErrInvalidSegment) {
		t.Fatalf("expected ErrInvalidSegment, got %v", err)
	}
}

func TestConsumerTenantIDAccessor(t *testing.T) {
	c, _ := NewTenantConsumer(newFakeSubscriber(), mse)
	if c.TenantID() != mse {
		t.Fatalf("TenantID() = %q, want %q", c.TenantID(), mse)
	}
}

func TestConsumerSubscribeBadDomain(t *testing.T) {
	c, _ := NewTenantConsumer(newFakeSubscriber(), ace)
	if err := c.Subscribe("BAD", "executed", nil); !errors.Is(err, ErrInvalidSegment) {
		t.Fatalf("expected ErrInvalidSegment, got %v", err)
	}
}

// ── Isolation property: two tenants never see each other's topics ─────────

func TestTenantIsolationEndToEnd(t *testing.T) {
	sub := newFakeSubscriber()
	aceC, _ := NewTenantConsumer(sub, ace)
	mseC, _ := NewTenantConsumer(sub, mse)

	var aceSeen, mseSeen int
	aceC.Subscribe("trades", "executed", func(ctx context.Context, k string, v []byte) error { aceSeen++; return nil })
	mseC.Subscribe("trades", "executed", func(ctx context.Context, k string, v []byte) error { mseSeen++; return nil })

	aceTopic := "ace-commodities.trades.executed"
	mseTopic := "mse-equities.trades.executed"

	// Correct routing.
	if err := sub.deliver(aceTopic, Delivery{Topic: aceTopic, Key: "1"}); err != nil {
		t.Fatal(err)
	}
	if err := sub.deliver(mseTopic, Delivery{Topic: mseTopic, Key: "2"}); err != nil {
		t.Fatal(err)
	}
	if aceSeen != 1 || mseSeen != 1 {
		t.Fatalf("expected 1 each, got ace=%d mse=%d", aceSeen, mseSeen)
	}

	// Attempt to feed mse data to the ace handler — must be rejected.
	if err := sub.deliver(aceTopic, Delivery{Topic: mseTopic, Key: "3"}); !errors.Is(err, ErrCrossTenant) {
		t.Fatalf("expected cross-tenant rejection, got %v", err)
	}
	if aceSeen != 1 {
		t.Fatalf("ace handler leaked mse data: aceSeen=%d", aceSeen)
	}
}
