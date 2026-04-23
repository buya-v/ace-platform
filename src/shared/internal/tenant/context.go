// Package tenant provides tenant context propagation for GarudaX services.
//
// Every inbound request must resolve to a TenantID before any domain logic
// runs. No service accepts traffic with an unresolved tenant (GarudaX_Strategy_Directive §3.3).
//
// Usage:
//
//	// In middleware (after validation):
//	ctx = tenant.WithTenant(ctx, tenant.TenantID("ace-commodities"))
//
//	// In domain logic:
//	id := tenant.MustTenant(ctx) // panics if missing — intentional
//
//	// Or when missing tenant is a non-fatal condition:
//	id, ok := tenant.TenantFromContext(ctx)
package tenant

import (
	"context"
	"fmt"
)

// TenantID is the canonical type for a GarudaX tenant identifier.
// Format: lowercase slug with hyphens, e.g. "ace-commodities", "mse-equities".
// Immutable after creation per platform-architecture §2.2.
type TenantID string

// String implements fmt.Stringer.
func (t TenantID) String() string {
	return string(t)
}

// contextKey is unexported so no external package can construct or collide
// with this key type.
type contextKey struct{}

// WithTenant returns a new context that carries the given TenantID.
// This is the only way to inject a tenant into a context; the key type is
// unexported so callers cannot bypass validation by constructing keys directly.
func WithTenant(ctx context.Context, id TenantID) context.Context {
	return context.WithValue(ctx, contextKey{}, id)
}

// TenantFromContext extracts the TenantID stored by WithTenant.
// Returns (id, true) when a tenant is present, ("", false) otherwise.
// Use MustTenant when the absence of a tenant is a programming error.
func TenantFromContext(ctx context.Context) (TenantID, bool) {
	id, ok := ctx.Value(contextKey{}).(TenantID)
	return id, ok
}

// MustTenant extracts the TenantID from ctx and panics if none is present.
//
// A service that reaches domain logic without a tenant context has a bug —
// this panic surfaces that bug immediately rather than silently allowing
// cross-tenant data access (GarudaX_Strategy_Directive §2.1, §3.3).
func MustTenant(ctx context.Context) TenantID {
	id, ok := TenantFromContext(ctx)
	if !ok {
		panic(fmt.Sprintf("tenant.MustTenant: no tenant in context — " +
			"TenantMiddleware must run before any domain handler"))
	}
	return id
}
