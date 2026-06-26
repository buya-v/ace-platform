package tenant

// This file makes the shared module's internal tenant package a thin alias over
// the canonical, zero-dependency tenant module (github.com/garudax-platform/tenant).
//
// The core tenant primitives (TenantID, the context key, WithTenant,
// TenantFromContext, MustTenant, HeaderName, TenantMiddleware) now live in that
// standalone module so every GarudaX service can adopt them via a go.mod replace
// without pulling in the heavier `shared` module (pgx, grpc). The gRPC
// interceptors in grpc.go remain here because they depend on google.golang.org/grpc,
// which the standalone core module deliberately avoids.
//
// Re-exporting via type aliases (TenantID = core.TenantID) means the context key
// is shared: a tenant injected by the HTTP/gRPC enforcement points and a tenant
// read by domain logic resolve to one and the same value.

import (
	"context"

	core "github.com/garudax-platform/tenant"
)

// TenantID is the canonical tenant identifier (alias of the core type).
type TenantID = core.TenantID

// HeaderName is the canonical HTTP header that carries the tenant identifier.
const HeaderName = core.HeaderName

// WithTenant injects a validated TenantID into ctx using the shared context key.
func WithTenant(ctx context.Context, id TenantID) context.Context {
	return core.WithTenant(ctx, id)
}

// TenantFromContext extracts the TenantID stored by WithTenant.
func TenantFromContext(ctx context.Context) (TenantID, bool) {
	return core.TenantFromContext(ctx)
}

// MustTenant extracts the TenantID from ctx and panics if none is present.
func MustTenant(ctx context.Context) TenantID {
	return core.MustTenant(ctx)
}

// TenantMiddleware returns the shared HTTP middleware that enforces tenant
// context on every request except the health/metrics bypass paths.
var TenantMiddleware = core.TenantMiddleware

// buildTenantSet converts a whitelist slice into an O(1) lookup set. Kept local
// (unexported) because grpc.go relies on it; mirrors the core implementation.
func buildTenantSet(validTenants []string) map[string]bool {
	m := make(map[string]bool, len(validTenants))
	for _, t := range validTenants {
		m[t] = true
	}
	return m
}
