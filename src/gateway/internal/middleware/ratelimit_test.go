package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTokenBucketAllow(t *testing.T) {
	rl := NewRateLimiter(&RateLimitConfig{Rate: 10, Burst: 2})

	// Should allow first 2 (burst)
	ok1, _, _ := rl.Allow("user1")
	ok2, _, _ := rl.Allow("user1")
	if !ok1 || !ok2 {
		t.Error("expected first 2 requests to be allowed (burst)")
	}

	// Third should be denied (burst exhausted, not enough time for refill)
	ok3, _, _ := rl.Allow("user1")
	if ok3 {
		t.Error("expected third request to be denied")
	}

	// Different key should be allowed
	ok4, _, _ := rl.Allow("user2")
	if !ok4 {
		t.Error("expected different user to be allowed")
	}
}

func TestRateLimitMiddleware(t *testing.T) {
	group := &RateLimitGroup{
		limiters: map[string]*RateLimiter{
			"test":    NewRateLimiter(&RateLimitConfig{Rate: 10, Burst: 1}),
			"default": NewRateLimiter(&RateLimitConfig{Rate: 100, Burst: 200}),
		},
	}

	handler := RateLimit(
		group,
		func(r *http.Request) string { return "test" },
		func(r *http.Request) string { return "testuser" },
	)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))

	// First request should pass
	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Errorf("first request: status = %d, want 200", rec.Code)
	}
	if rec.Header().Get("X-RateLimit-Limit") == "" {
		t.Error("expected X-RateLimit-Limit header")
	}

	// Second should be rate limited
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req)

	if rec2.Code != 429 {
		t.Errorf("second request: status = %d, want 429", rec2.Code)
	}
	if rec2.Header().Get("Retry-After") != "1" {
		t.Errorf("Retry-After = %q, want %q", rec2.Header().Get("Retry-After"), "1")
	}
}

func TestRateLimitGroupDefaults(t *testing.T) {
	group := NewRateLimitGroup()

	// All expected groups should exist
	groups := []string{"order_submit", "order_cancel", "order_query", "market_data",
		"market_public", "compliance", "admin", "auth", "default"}

	for _, g := range groups {
		rl := group.GetLimiter(g)
		if rl == nil {
			t.Errorf("expected limiter for group %q", g)
		}
	}

	// Unknown group should fall back to default
	rl := group.GetLimiter("unknown_group")
	if rl == nil {
		t.Error("expected default limiter for unknown group")
	}
}
