package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"context"

	"github.com/redis/go-redis/v9"
)

func testRedisRateLimiter(t *testing.T) *RedisRateLimiter {
	t.Helper()
	rdb := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
		DB:   15,
	})
	ctx := context.Background()
	if err := rdb.Ping(ctx).Err(); err != nil {
		rdb.Close()
		t.Skip("Redis not available, skipping Redis rate limiter tests")
	}
	rdb.FlushDB(ctx)

	fallback := &RateLimitGroup{
		limiters: map[string]*RateLimiter{
			"test":    NewRateLimiter(&RateLimitConfig{Rate: 5, Burst: 10}),
			"strict":  NewRateLimiter(&RateLimitConfig{Rate: 1, Burst: 2}),
			"default": NewRateLimiter(&RateLimitConfig{Rate: 100, Burst: 200}),
		},
	}

	rl := NewRedisRateLimiterFromClient(rdb, fallback, time.Minute)
	t.Cleanup(func() { rl.Close() })
	return rl
}

func TestRedisRateLimiterAllow(t *testing.T) {
	rl := testRedisRateLimiter(t)

	cfg := &RateLimitConfig{Rate: 5, Burst: 10}

	// 5 req/sec * 60 sec window = 300 limit per window
	for i := 0; i < 300; i++ {
		allowed, _, _ := rl.allow("test", "user1", cfg)
		if !allowed {
			t.Fatalf("request %d should be allowed (limit 300/window)", i+1)
		}
	}

	// 301st should be denied
	allowed, _, _ := rl.allow("test", "user1", cfg)
	if allowed {
		t.Error("request 301 should be denied")
	}
}

func TestRedisRateLimiterDifferentKeys(t *testing.T) {
	rl := testRedisRateLimiter(t)

	cfg := &RateLimitConfig{Rate: 1, Burst: 2}
	// limit = 1 * 60 = 60

	// Fill up user1
	for i := 0; i < 60; i++ {
		rl.allow("strict", "user1", cfg)
	}

	// user1 should be denied
	allowed, _, _ := rl.allow("strict", "user1", cfg)
	if allowed {
		t.Error("user1 should be denied after exhausting limit")
	}

	// user2 should still be allowed
	allowed2, _, _ := rl.allow("strict", "user2", cfg)
	if !allowed2 {
		t.Error("user2 should be allowed (separate key)")
	}
}

func TestRedisRateLimitMiddleware(t *testing.T) {
	rl := testRedisRateLimiter(t)

	// Override with a very strict config — 1 req/sec = 60/min window
	rl.fallback = &RateLimitGroup{
		limiters: map[string]*RateLimiter{
			"test":    NewRateLimiter(&RateLimitConfig{Rate: 0.0167, Burst: 1}), // ~1 per minute
			"default": NewRateLimiter(&RateLimitConfig{Rate: 100, Burst: 200}),
		},
	}

	handler := RedisRateLimit(
		rl,
		func(r *http.Request) string { return "test" },
		func(r *http.Request) string { return "testuser" },
	)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))

	// First request should pass (limit = 0.0167*60 = 1)
	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Errorf("first request: status = %d, want 200", rec.Code)
	}
	if rec.Header().Get("X-RateLimit-Limit") == "" {
		t.Error("expected X-RateLimit-Limit header")
	}

	// Second request should be rate limited
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req)
	if rec2.Code != 429 {
		t.Errorf("second request: status = %d, want 429", rec2.Code)
	}
	if rec2.Header().Get("Retry-After") == "" {
		t.Error("expected Retry-After header")
	}
}

func TestRedisRateLimitHeaders(t *testing.T) {
	rl := testRedisRateLimiter(t)

	handler := RedisRateLimit(
		rl,
		func(r *http.Request) string { return "test" },
		func(r *http.Request) string { return "headercheck" },
	)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Verify rate limit headers are present
	for _, header := range []string{"X-RateLimit-Limit", "X-RateLimit-Remaining", "X-RateLimit-Reset"} {
		if rec.Header().Get(header) == "" {
			t.Errorf("expected %s header to be set", header)
		}
	}
}
