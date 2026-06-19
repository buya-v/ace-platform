// Package kafka provides a tenant-aware routing layer over any underlying
// Kafka transport for the GarudaX multi-tenant platform.
//
// Every GarudaX topic follows the canonical naming convention:
//
//	{tenant_id}.{domain}.{event}
//
// e.g. "ace-commodities.trades.executed" or "mse-equities.clearing.novated".
//
// The wrappers in this package make the tenant prefix non-optional and
// non-forgeable: producers derive the tenant from the request context (or are
// bound to a single tenant at construction), and consumers refuse to deliver a
// record whose topic prefix does not match the tenant they are scoped to.
// This closes the cross-tenant message-leakage hole that arises when callers
// assemble topic strings by hand.
//
// Platform invariant (GarudaX_Strategy_Directive §2.1, §3.3):
//
//	Tenant ID is never optional. No service produces to, or consumes from,
//	a topic it cannot prove belongs to the active tenant.
package kafka

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/garudax-platform/shared/internal/tenant"
)

// Sentinel errors returned by this package. Callers may use errors.Is to
// branch on them.
var (
	// ErrNoTenant is returned when an operation needs a tenant but none is
	// present in the context and the producer/consumer is not bound to one.
	ErrNoTenant = errors.New("kafka: no tenant in context")

	// ErrCrossTenant is returned when an operation would cross a tenant
	// boundary — e.g. publishing under a tenant that differs from the
	// context tenant, or consuming a record whose topic belongs to a
	// different tenant. This is a hard security boundary, never a warning.
	ErrCrossTenant = errors.New("kafka: cross-tenant routing rejected")

	// ErrInvalidTopic is returned when a topic string does not match the
	// {tenant_id}.{domain}.{event} convention.
	ErrInvalidTopic = errors.New("kafka: invalid topic")

	// ErrInvalidSegment is returned when a tenant_id, domain, or event
	// segment is empty or contains characters outside the allowed slug set.
	ErrInvalidSegment = errors.New("kafka: invalid topic segment")
)

// segmentPattern constrains every topic segment to a lowercase slug:
// letters/digits separated by single hyphens, no leading/trailing hyphen.
// This matches existing GarudaX tenant ids ("ace-commodities"), domains
// ("market-data"), and events ("call-issued").
var segmentPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// validateSegment returns nil if s is a legal topic segment.
func validateSegment(kind, s string) error {
	if s == "" {
		return fmt.Errorf("%w: %s is empty", ErrInvalidSegment, kind)
	}
	if strings.Contains(s, ".") {
		return fmt.Errorf("%w: %s %q contains a dot (dots separate segments)", ErrInvalidSegment, kind, s)
	}
	if !segmentPattern.MatchString(s) {
		return fmt.Errorf("%w: %s %q must be a lowercase slug ([a-z0-9] with single hyphens)", ErrInvalidSegment, kind, s)
	}
	return nil
}

// Topic is the parsed, validated representation of a GarudaX topic name.
type Topic struct {
	TenantID tenant.TenantID
	Domain   string
	Event    string
}

// String renders the topic back into its canonical "{tenant}.{domain}.{event}"
// wire form.
func (t Topic) String() string {
	return string(t.TenantID) + "." + t.Domain + "." + t.Event
}

// BuildTopic assembles a canonical topic name from its three segments,
// validating each. It is the only sanctioned way to construct a topic string;
// hand-concatenation is what this package exists to prevent.
func BuildTopic(tenantID tenant.TenantID, domain, event string) (string, error) {
	if err := validateSegment("tenant_id", string(tenantID)); err != nil {
		return "", err
	}
	if err := validateSegment("domain", domain); err != nil {
		return "", err
	}
	if err := validateSegment("event", event); err != nil {
		return "", err
	}
	return string(tenantID) + "." + domain + "." + event, nil
}

// ParseTopic decomposes a topic string into its segments and validates the
// shape. The domain may itself be a multi-word slug, but the tenant and event
// are always the first and last dot-separated fields respectively.
//
// Examples:
//
//	"ace-commodities.trades.executed"      -> {ace-commodities, trades, executed}
//	"mse-equities.market-data.tick"        -> {mse-equities, market-data, tick}
func ParseTopic(topic string) (Topic, error) {
	parts := strings.Split(topic, ".")
	if len(parts) < 3 {
		return Topic{}, fmt.Errorf("%w: %q must have form {tenant}.{domain}.{event}", ErrInvalidTopic, topic)
	}
	tenantID := parts[0]
	event := parts[len(parts)-1]
	domain := strings.Join(parts[1:len(parts)-1], ".")

	if err := validateSegment("tenant_id", tenantID); err != nil {
		return Topic{}, err
	}
	// domain may contain dots only if it were multi-level; we disallow that to
	// keep a single, predictable convention. Each inner field must be a slug.
	for _, d := range strings.Split(domain, ".") {
		if err := validateSegment("domain", d); err != nil {
			return Topic{}, err
		}
	}
	if err := validateSegment("event", event); err != nil {
		return Topic{}, err
	}
	return Topic{TenantID: tenant.TenantID(tenantID), Domain: domain, Event: event}, nil
}

// TopicBelongsTo reports whether topic is owned by tenantID, i.e. its first
// segment equals the tenant id. It returns false for malformed topics. This is
// the predicate used to gate every consume.
func TopicBelongsTo(topic string, tenantID tenant.TenantID) bool {
	parsed, err := ParseTopic(topic)
	if err != nil {
		return false
	}
	return parsed.TenantID == tenantID
}

// ── Transport interfaces ─────────────────────────────────────────────────
//
// These minimal interfaces let the wrappers sit on top of any concrete Kafka
// client (the in-process ChannelProducer/Consumer used in tests and local
// dev, or a real wire-protocol adapter in production) without depending on it.

// Publisher is the underlying transport a TenantProducer writes through.
type Publisher interface {
	// Publish sends value to topic with the given partition key.
	Publish(ctx context.Context, topic, key string, value []byte) error
}

// Delivery is a record handed to a subscriber's handler. It carries the topic
// so the routing layer can re-verify tenant ownership on the consume path.
type Delivery struct {
	Topic string
	Key   string
	Value []byte
}

// RawHandler processes a raw delivery from the underlying transport.
type RawHandler func(ctx context.Context, d Delivery) error

// Subscriber is the underlying transport a TenantConsumer reads through.
type Subscriber interface {
	// Subscribe registers handler for topic. The transport must invoke
	// handler with the originating topic populated on each Delivery.
	Subscribe(topic string, handler RawHandler) error
}

// Handler is the tenant-scoped handler signature exposed to callers. The
// context is guaranteed to carry the consumer's tenant, so downstream domain
// logic can call tenant.MustTenant(ctx) safely.
type Handler func(ctx context.Context, key string, value []byte) error

// ── Producer ─────────────────────────────────────────────────────────────

// TenantProducer publishes events under the {tenant}.{domain}.{event}
// convention, deriving the tenant from context and refusing cross-tenant
// writes.
type TenantProducer struct {
	pub     Publisher
	bound   tenant.TenantID
	isBound bool
}

// NewTenantProducer returns a producer that derives the tenant from the
// context passed to each Publish call. Use this for services that handle
// multiple tenants on shared infrastructure.
func NewTenantProducer(pub Publisher) *TenantProducer {
	return &TenantProducer{pub: pub}
}

// NewBoundProducer returns a producer locked to a single tenant. Any Publish
// whose context carries a different tenant is rejected with ErrCrossTenant.
// Use this for per-tenant deployments where mixing is always a bug.
func NewBoundProducer(pub Publisher, tenantID tenant.TenantID) (*TenantProducer, error) {
	if err := validateSegment("tenant_id", string(tenantID)); err != nil {
		return nil, err
	}
	return &TenantProducer{pub: pub, bound: tenantID, isBound: true}, nil
}

// resolveTenant determines the effective tenant for an operation, enforcing
// the binding and context-consistency rules.
func (p *TenantProducer) resolveTenant(ctx context.Context) (tenant.TenantID, error) {
	ctxID, hasCtx := tenant.TenantFromContext(ctx)

	switch {
	case p.isBound && hasCtx && ctxID != p.bound:
		// Context tenant contradicts the producer's binding: a leak attempt.
		return "", fmt.Errorf("%w: producer bound to %q but context carries %q",
			ErrCrossTenant, p.bound, ctxID)
	case p.isBound:
		return p.bound, nil
	case hasCtx:
		return ctxID, nil
	default:
		return "", ErrNoTenant
	}
}

// Publish builds the topic "{tenant}.{domain}.{event}" — with the tenant taken
// from context (or the producer binding) — and writes value through the
// underlying transport. The caller never supplies the tenant prefix, so it can
// neither omit nor spoof it.
func (p *TenantProducer) Publish(ctx context.Context, domain, event, key string, value []byte) error {
	tenantID, err := p.resolveTenant(ctx)
	if err != nil {
		return err
	}
	topic, err := BuildTopic(tenantID, domain, event)
	if err != nil {
		return err
	}
	// Ensure the context handed to the transport carries the resolved tenant,
	// so downstream interceptors (logging, tracing) see a consistent value.
	ctx = tenant.WithTenant(ctx, tenantID)
	return p.pub.Publish(ctx, topic, key, value)
}

// ── Consumer ─────────────────────────────────────────────────────────────

// TenantConsumer subscribes to topics for exactly one tenant and guarantees
// that handlers only ever observe records belonging to that tenant.
type TenantConsumer struct {
	sub      Subscriber
	tenantID tenant.TenantID
}

// NewTenantConsumer returns a consumer scoped to tenantID. Every Subscribe
// targets only that tenant's topics, and every delivered record is re-checked
// against the tenant prefix before reaching the handler.
func NewTenantConsumer(sub Subscriber, tenantID tenant.TenantID) (*TenantConsumer, error) {
	if err := validateSegment("tenant_id", string(tenantID)); err != nil {
		return nil, err
	}
	return &TenantConsumer{sub: sub, tenantID: tenantID}, nil
}

// TenantID returns the tenant this consumer is scoped to.
func (c *TenantConsumer) TenantID() tenant.TenantID { return c.tenantID }

// Subscribe registers handler for the "{tenant}.{domain}.{event}" topic of
// this consumer's tenant. The handler receives a context already carrying the
// tenant, and is never invoked for a record whose topic belongs to another
// tenant (such a record is dropped and an error returned to the transport so
// it can be sent to a DLQ rather than silently acknowledged).
func (c *TenantConsumer) Subscribe(domain, event string, handler Handler) error {
	topic, err := BuildTopic(c.tenantID, domain, event)
	if err != nil {
		return err
	}
	return c.sub.Subscribe(topic, c.guard(handler))
}

// guard wraps a caller handler with the cross-tenant verification and tenant
// context injection.
func (c *TenantConsumer) guard(handler Handler) RawHandler {
	return func(ctx context.Context, d Delivery) error {
		if !TopicBelongsTo(d.Topic, c.tenantID) {
			// Defense in depth: a misrouted or wildcard delivery for the
			// wrong tenant must never reach domain logic.
			return fmt.Errorf("%w: consumer for %q received record on %q",
				ErrCrossTenant, c.tenantID, d.Topic)
		}
		ctx = tenant.WithTenant(ctx, c.tenantID)
		return handler(ctx, d.Key, d.Value)
	}
}
