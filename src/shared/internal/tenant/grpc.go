package tenant

import (
	"context"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// MetadataKey is the gRPC metadata key that carries the tenant identifier.
//
// gRPC normalises all metadata keys to lower-case on the wire, so the HTTP
// header HeaderName ("X-GarudaX-Tenant") maps to this key for gRPC calls.
// The two transports therefore agree on a single logical header name.
const MetadataKey = "x-garudax-tenant"

// bypassMethodPrefixes are fully-qualified gRPC method prefixes that are
// exempt from tenant enforcement: the standard health-checking and server
// reflection services, which must be reachable by infrastructure probes and
// tooling without a tenant context (mirrors the HTTP healthPaths bypass).
var bypassMethodPrefixes = []string{
	"/grpc.health.v1.Health/",
	"/grpc.reflection.",
}

// isBypassMethod reports whether fullMethod (e.g. "/pkg.Service/Method") is
// exempt from tenant enforcement.
func isBypassMethod(fullMethod string) bool {
	for _, p := range bypassMethodPrefixes {
		if strings.HasPrefix(fullMethod, p) {
			return true
		}
	}
	return false
}

// tenantFromMetadata extracts and validates the tenant id carried in inbound
// gRPC metadata against tenantMap. It returns a gRPC status error mirroring the
// HTTP middleware's error codes:
//
//   - missing/empty header  → codes.Unauthenticated  ("TENANT_REQUIRED")
//   - unregistered tenant   → codes.PermissionDenied ("UNKNOWN_TENANT")
func tenantFromMetadata(ctx context.Context, tenantMap map[string]bool) (TenantID, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", status.Error(codes.Unauthenticated,
			"TENANT_REQUIRED: X-GarudaX-Tenant metadata is required")
	}

	values := md.Get(MetadataKey)
	if len(values) == 0 || values[0] == "" {
		return "", status.Error(codes.Unauthenticated,
			"TENANT_REQUIRED: X-GarudaX-Tenant metadata is required")
	}

	tenantID := values[0]
	if !tenantMap[tenantID] {
		return "", status.Errorf(codes.PermissionDenied,
			"UNKNOWN_TENANT: tenant '%s' is not registered", tenantID)
	}

	return TenantID(tenantID), nil
}

// GRPCInterceptors enforces tenant context on inbound gRPC calls. It is the
// gRPC counterpart of TenantMiddleware: it reads the tenant id from request
// metadata, validates it against a whitelist, and injects the resolved
// TenantID into the handler's context (GarudaX_Strategy_Directive §3.3 — the
// middleware "rejects any call without a tenant context").
//
// Construct one with NewGRPCInterceptors and register both Unary() and
// Stream() on the server:
//
//	ti := tenant.NewGRPCInterceptors([]string{"ace-commodities", "mse-equities"})
//	grpc.NewServer(
//		grpc.UnaryInterceptor(ti.Unary()),
//		grpc.StreamInterceptor(ti.Stream()),
//	)
type GRPCInterceptors struct {
	tenantMap map[string]bool
}

// NewGRPCInterceptors returns a GRPCInterceptors that validates inbound tenant
// metadata against validTenants. An empty whitelist rejects every call, exactly
// as the HTTP middleware does.
func NewGRPCInterceptors(validTenants []string) *GRPCInterceptors {
	return &GRPCInterceptors{tenantMap: buildTenantSet(validTenants)}
}

// Unary returns a grpc.UnaryServerInterceptor that enforces tenant context.
// On success it calls the handler with a context carrying the resolved tenant;
// on failure it returns a gRPC status error and does not invoke the handler.
func (g *GRPCInterceptors) Unary() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		if isBypassMethod(info.FullMethod) {
			return handler(ctx, req)
		}

		id, err := tenantFromMetadata(ctx, g.tenantMap)
		if err != nil {
			return nil, err
		}

		return handler(WithTenant(ctx, id), req)
	}
}

// Stream returns a grpc.StreamServerInterceptor that enforces tenant context.
// The stream's context is replaced with one carrying the resolved tenant so
// downstream handlers and observability helpers see the tenant via
// ServerStream.Context().
func (g *GRPCInterceptors) Stream() grpc.StreamServerInterceptor {
	return func(
		srv any,
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		if isBypassMethod(info.FullMethod) {
			return handler(srv, ss)
		}

		id, err := tenantFromMetadata(ss.Context(), g.tenantMap)
		if err != nil {
			return err
		}

		return handler(srv, &tenantServerStream{
			ServerStream: ss,
			ctx:          WithTenant(ss.Context(), id),
		})
	}
}

// tenantServerStream wraps a grpc.ServerStream to override Context() with one
// that carries the resolved tenant. grpc.ServerStream has no setter for its
// context, so wrapping is the standard way to propagate values into a stream.
type tenantServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

// Context returns the tenant-enriched context.
func (s *tenantServerStream) Context() context.Context { return s.ctx }

// UnaryClientInterceptor returns a grpc.UnaryClientInterceptor that propagates
// the tenant from the outgoing call's context into request metadata, so a
// downstream service's GRPCInterceptors can enforce it. When no tenant is
// present in the context the call proceeds unchanged — the receiving server is
// responsible for rejecting calls that require a tenant.
func UnaryClientInterceptor() grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply any,
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		return invoker(withOutgoingTenant(ctx), method, req, reply, cc, opts...)
	}
}

// StreamClientInterceptor is the streaming counterpart of
// UnaryClientInterceptor: it propagates the tenant from ctx into outgoing
// stream metadata.
func StreamClientInterceptor() grpc.StreamClientInterceptor {
	return func(
		ctx context.Context,
		desc *grpc.StreamDesc,
		cc *grpc.ClientConn,
		method string,
		streamer grpc.Streamer,
		opts ...grpc.CallOption,
	) (grpc.ClientStream, error) {
		return streamer(withOutgoingTenant(ctx), desc, cc, method, opts...)
	}
}

// withOutgoingTenant appends the tenant carried in ctx (if any) to the outgoing
// gRPC metadata. It is a no-op when ctx carries no tenant.
func withOutgoingTenant(ctx context.Context) context.Context {
	id, ok := TenantFromContext(ctx)
	if !ok {
		return ctx
	}
	return metadata.AppendToOutgoingContext(ctx, MetadataKey, id.String())
}
