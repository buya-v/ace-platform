package tenant

import (
	"context"
	"log/slog"
)

// TenantLogger returns a copy of base enriched with a "tenant_id" attribute
// sourced from ctx. If no tenant is present in ctx, the logger is returned
// unchanged so callers are not penalised on health/bypass paths.
//
// This satisfies GarudaX_Strategy_Directive §3.3:
// "every metric, log, and trace carries tenant_id as a required label."
func TenantLogger(ctx context.Context, base *slog.Logger) *slog.Logger {
	id, ok := TenantFromContext(ctx)
	if !ok {
		return base
	}
	return base.With(slog.String("tenant_id", id.String()))
}

// TenantMetricLabels returns a label map suitable for attaching to Prometheus
// metrics or OpenTelemetry attributes. The map always contains "tenant_id";
// when no tenant is present the value is the empty string so callers can
// unconditionally include it without branching.
//
// Usage:
//
//	labels := tenant.TenantMetricLabels(ctx)
//	counter.With(prometheus.Labels(labels)).Inc()
func TenantMetricLabels(ctx context.Context) map[string]string {
	id, _ := TenantFromContext(ctx) // zero value "" when missing
	return map[string]string{
		"tenant_id": id.String(),
	}
}
