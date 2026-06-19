package tenant_test

import (
	"context"
	"testing"

	"github.com/garudax-platform/shared/internal/tenant"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// grpcValidTenants mirrors the HTTP test whitelist.
var grpcValidTenants = []string{"ace-commodities", "mse-equities"}

// incomingCtx builds a context carrying the given tenant metadata value, as a
// server would receive it. Passing "" omits the key entirely.
func incomingCtx(tenantID string) context.Context {
	if tenantID == "" {
		return metadata.NewIncomingContext(context.Background(), metadata.MD{})
	}
	return metadata.NewIncomingContext(
		context.Background(),
		metadata.Pairs(tenant.MetadataKey, tenantID),
	)
}

// --- Unary server interceptor ---

func TestGRPCUnary_ValidTenantInjectsContext(t *testing.T) {
	ti := tenant.NewGRPCInterceptors(grpcValidTenants)
	interceptor := ti.Unary()

	var seen tenant.TenantID
	var ok bool
	handler := func(ctx context.Context, _ any) (any, error) {
		seen, ok = tenant.TenantFromContext(ctx)
		return "response", nil
	}

	resp, err := interceptor(
		incomingCtx("ace-commodities"),
		nil,
		&grpc.UnaryServerInfo{FullMethod: "/exchange.Matching/Submit"},
		handler,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != "response" {
		t.Fatalf("handler response not returned, got %v", resp)
	}
	if !ok {
		t.Fatal("tenant was not injected into handler context")
	}
	if seen != "ace-commodities" {
		t.Fatalf("expected ace-commodities, got %q", seen)
	}
}

func TestGRPCUnary_MissingMetadataIsUnauthenticated(t *testing.T) {
	ti := tenant.NewGRPCInterceptors(grpcValidTenants)
	called := false
	handler := func(context.Context, any) (any, error) { called = true; return nil, nil }

	_, err := ti.Unary()(
		incomingCtx(""),
		nil,
		&grpc.UnaryServerInfo{FullMethod: "/exchange.Matching/Submit"},
		handler,
	)
	if called {
		t.Fatal("handler must not run when tenant metadata is missing")
	}
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("expected Unauthenticated, got %v", status.Code(err))
	}
}

func TestGRPCUnary_NoMetadataAtAllIsUnauthenticated(t *testing.T) {
	ti := tenant.NewGRPCInterceptors(grpcValidTenants)
	// context.Background() has no metadata at all (FromIncomingContext ok=false).
	_, err := ti.Unary()(
		context.Background(),
		nil,
		&grpc.UnaryServerInfo{FullMethod: "/exchange.Matching/Submit"},
		func(context.Context, any) (any, error) { return nil, nil },
	)
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("expected Unauthenticated, got %v", status.Code(err))
	}
}

func TestGRPCUnary_UnknownTenantIsPermissionDenied(t *testing.T) {
	ti := tenant.NewGRPCInterceptors(grpcValidTenants)
	called := false
	handler := func(context.Context, any) (any, error) { called = true; return nil, nil }

	_, err := ti.Unary()(
		incomingCtx("rogue-exchange"),
		nil,
		&grpc.UnaryServerInfo{FullMethod: "/exchange.Matching/Submit"},
		handler,
	)
	if called {
		t.Fatal("handler must not run for an unregistered tenant")
	}
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("expected PermissionDenied, got %v", status.Code(err))
	}
}

func TestGRPCUnary_HealthMethodBypassesEnforcement(t *testing.T) {
	ti := tenant.NewGRPCInterceptors(grpcValidTenants)
	called := false
	handler := func(ctx context.Context, _ any) (any, error) {
		called = true
		// No tenant should be injected on the bypass path.
		if _, ok := tenant.TenantFromContext(ctx); ok {
			t.Fatal("tenant unexpectedly injected on health bypass path")
		}
		return "ok", nil
	}

	_, err := ti.Unary()(
		incomingCtx(""), // no tenant metadata
		nil,
		&grpc.UnaryServerInfo{FullMethod: "/grpc.health.v1.Health/Check"},
		handler,
	)
	if err != nil {
		t.Fatalf("health check must not be rejected: %v", err)
	}
	if !called {
		t.Fatal("handler must run for health bypass path")
	}
}

func TestGRPCUnary_EmptyWhitelistRejectsEverything(t *testing.T) {
	ti := tenant.NewGRPCInterceptors(nil)
	_, err := ti.Unary()(
		incomingCtx("ace-commodities"),
		nil,
		&grpc.UnaryServerInfo{FullMethod: "/exchange.Matching/Submit"},
		func(context.Context, any) (any, error) { return nil, nil },
	)
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("expected PermissionDenied with empty whitelist, got %v", status.Code(err))
	}
}

func TestGRPCUnary_ValidationIsCaseSensitive(t *testing.T) {
	ti := tenant.NewGRPCInterceptors(grpcValidTenants)
	_, err := ti.Unary()(
		incomingCtx("ACE-COMMODITIES"),
		nil,
		&grpc.UnaryServerInfo{FullMethod: "/exchange.Matching/Submit"},
		func(context.Context, any) (any, error) { return nil, nil },
	)
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("expected PermissionDenied for case-mismatched tenant, got %v", status.Code(err))
	}
}

// --- Stream server interceptor ---

// fakeServerStream is a minimal grpc.ServerStream whose Context() is controllable.
type fakeServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (f *fakeServerStream) Context() context.Context { return f.ctx }

func TestGRPCStream_ValidTenantInjectsContext(t *testing.T) {
	ti := tenant.NewGRPCInterceptors(grpcValidTenants)

	var seen tenant.TenantID
	handler := func(_ any, ss grpc.ServerStream) error {
		seen = tenant.MustTenant(ss.Context())
		return nil
	}

	err := ti.Stream()(
		nil,
		&fakeServerStream{ctx: incomingCtx("mse-equities")},
		&grpc.StreamServerInfo{FullMethod: "/marketdata.Stream/Subscribe"},
		handler,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if seen != "mse-equities" {
		t.Fatalf("expected mse-equities in stream context, got %q", seen)
	}
}

func TestGRPCStream_MissingTenantRejected(t *testing.T) {
	ti := tenant.NewGRPCInterceptors(grpcValidTenants)
	called := false
	handler := func(any, grpc.ServerStream) error { called = true; return nil }

	err := ti.Stream()(
		nil,
		&fakeServerStream{ctx: incomingCtx("")},
		&grpc.StreamServerInfo{FullMethod: "/marketdata.Stream/Subscribe"},
		handler,
	)
	if called {
		t.Fatal("stream handler must not run without a tenant")
	}
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("expected Unauthenticated, got %v", status.Code(err))
	}
}

func TestGRPCStream_HealthBypass(t *testing.T) {
	ti := tenant.NewGRPCInterceptors(grpcValidTenants)
	called := false
	handler := func(any, grpc.ServerStream) error { called = true; return nil }

	err := ti.Stream()(
		nil,
		&fakeServerStream{ctx: incomingCtx("")},
		&grpc.StreamServerInfo{FullMethod: "/grpc.health.v1.Health/Watch"},
		handler,
	)
	if err != nil {
		t.Fatalf("health watch must not be rejected: %v", err)
	}
	if !called {
		t.Fatal("handler must run for stream health bypass path")
	}
}

// --- Client interceptors: outgoing propagation ---

func TestUnaryClientInterceptor_PropagatesTenant(t *testing.T) {
	ctx := tenant.WithTenant(context.Background(), tenant.TenantID("ace-commodities"))

	var got []string
	invoker := func(ctx context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		md, _ := metadata.FromOutgoingContext(ctx)
		got = md.Get(tenant.MetadataKey)
		return nil
	}

	err := tenant.UnaryClientInterceptor()(ctx, "/exchange.Matching/Submit", nil, nil, nil, invoker)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0] != "ace-commodities" {
		t.Fatalf("expected outgoing tenant metadata [ace-commodities], got %v", got)
	}
}

func TestUnaryClientInterceptor_NoTenantNoMetadata(t *testing.T) {
	var hadKey bool
	invoker := func(ctx context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		md, ok := metadata.FromOutgoingContext(ctx)
		hadKey = ok && len(md.Get(tenant.MetadataKey)) > 0
		return nil
	}

	err := tenant.UnaryClientInterceptor()(context.Background(), "/exchange.Matching/Submit", nil, nil, nil, invoker)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hadKey {
		t.Fatal("no tenant metadata should be attached when context carries no tenant")
	}
}

func TestStreamClientInterceptor_PropagatesTenant(t *testing.T) {
	ctx := tenant.WithTenant(context.Background(), tenant.TenantID("mse-equities"))

	var got []string
	streamer := func(ctx context.Context, _ *grpc.StreamDesc, _ *grpc.ClientConn, _ string, _ ...grpc.CallOption) (grpc.ClientStream, error) {
		md, _ := metadata.FromOutgoingContext(ctx)
		got = md.Get(tenant.MetadataKey)
		return nil, nil
	}

	_, err := tenant.StreamClientInterceptor()(ctx, &grpc.StreamDesc{}, nil, "/marketdata.Stream/Subscribe", streamer)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0] != "mse-equities" {
		t.Fatalf("expected outgoing tenant metadata [mse-equities], got %v", got)
	}
}

// --- Round trip: client propagation feeds server enforcement ---

func TestGRPC_ClientToServerRoundTrip(t *testing.T) {
	// Simulate the wire: client interceptor writes outgoing metadata, which the
	// server reads from incoming metadata.
	clientCtx := tenant.WithTenant(context.Background(), tenant.TenantID("ace-commodities"))

	var serverSaw tenant.TenantID
	serverHandler := func(ctx context.Context, _ any) (any, error) {
		serverSaw = tenant.MustTenant(ctx)
		return nil, nil
	}
	server := tenant.NewGRPCInterceptors(grpcValidTenants).Unary()

	invoker := func(outCtx context.Context, method string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		// Transcribe outgoing → incoming as the transport would.
		md, _ := metadata.FromOutgoingContext(outCtx)
		inCtx := metadata.NewIncomingContext(context.Background(), md)
		_, err := server(inCtx, nil, &grpc.UnaryServerInfo{FullMethod: method}, serverHandler)
		return err
	}

	err := tenant.UnaryClientInterceptor()(clientCtx, "/exchange.Matching/Submit", nil, nil, nil, invoker)
	if err != nil {
		t.Fatalf("round trip failed: %v", err)
	}
	if serverSaw != "ace-commodities" {
		t.Fatalf("server resolved wrong tenant: %q", serverSaw)
	}
}
