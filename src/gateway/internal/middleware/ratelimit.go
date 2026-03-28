package middleware

import (
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/ace-platform/gateway/internal/types"
)

// RateLimitConfig defines rate limit parameters for an endpoint group.
type RateLimitConfig struct {
	Rate  float64 // tokens per second
	Burst int     // max burst
}

// tokenBucket implements a token bucket rate limiter.
type tokenBucket struct {
	mu         sync.Mutex
	tokens     float64
	maxTokens  float64
	rate       float64
	lastRefill time.Time
}

func newTokenBucket(rate float64, burst int) *tokenBucket {
	return &tokenBucket{
		tokens:     float64(burst),
		maxTokens:  float64(burst),
		rate:       rate,
		lastRefill: time.Now(),
	}
}

func (tb *tokenBucket) allow() (bool, float64, float64) {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(tb.lastRefill).Seconds()
	tb.tokens += elapsed * tb.rate
	if tb.tokens > tb.maxTokens {
		tb.tokens = tb.maxTokens
	}
	tb.lastRefill = now

	if tb.tokens < 1.0 {
		return false, 0, tb.maxTokens
	}

	tb.tokens--
	return true, tb.tokens, tb.maxTokens
}

// RateLimiter manages per-key rate limiting with token buckets.
type RateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*tokenBucket
	config  *RateLimitConfig
	nowFunc func() time.Time
}

// NewRateLimiter creates a new rate limiter.
func NewRateLimiter(cfg *RateLimitConfig) *RateLimiter {
	return &RateLimiter{
		buckets: make(map[string]*tokenBucket),
		config:  cfg,
		nowFunc: time.Now,
	}
}

func (rl *RateLimiter) getBucket(key string) *tokenBucket {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	b, ok := rl.buckets[key]
	if !ok {
		b = newTokenBucket(rl.config.Rate, rl.config.Burst)
		rl.buckets[key] = b
	}
	return b
}

// Allow checks if a request is allowed for the given key.
func (rl *RateLimiter) Allow(key string) (bool, float64, float64) {
	return rl.getBucket(key).allow()
}

// RateLimitGroup holds rate limiters keyed by endpoint group.
type RateLimitGroup struct {
	limiters map[string]*RateLimiter
}

// NewRateLimitGroup creates rate limiters for all endpoint groups.
func NewRateLimitGroup() *RateLimitGroup {
	return &RateLimitGroup{
		limiters: map[string]*RateLimiter{
			"order_submit":  NewRateLimiter(&RateLimitConfig{Rate: 50, Burst: 100}),
			"order_cancel":  NewRateLimiter(&RateLimitConfig{Rate: 100, Burst: 200}),
			"order_query":   NewRateLimiter(&RateLimitConfig{Rate: 100, Burst: 200}),
			"market_data":   NewRateLimiter(&RateLimitConfig{Rate: 200, Burst: 400}),
			"market_public": NewRateLimiter(&RateLimitConfig{Rate: 20, Burst: 40}),
			"compliance":    NewRateLimiter(&RateLimitConfig{Rate: 30, Burst: 60}),
			"admin":         NewRateLimiter(&RateLimitConfig{Rate: 10, Burst: 20}),
			"auth":          NewRateLimiter(&RateLimitConfig{Rate: 5, Burst: 10}),
			"default":       NewRateLimiter(&RateLimitConfig{Rate: 100, Burst: 200}),
		},
	}
}

// GetLimiter returns the rate limiter for a given group.
func (g *RateLimitGroup) GetLimiter(group string) *RateLimiter {
	if rl, ok := g.limiters[group]; ok {
		return rl
	}
	return g.limiters["default"]
}

// RateLimit creates rate limiting middleware.
// The groupFn determines which rate limit group a request belongs to.
// The keyFn determines the rate limit key (user ID or IP).
func RateLimit(group *RateLimitGroup, groupFn func(*http.Request) string, keyFn func(*http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			grp := groupFn(r)
			key := keyFn(r)
			rl := group.GetLimiter(grp)

			allowed, remaining, limit := rl.Allow(key)

			w.Header().Set("X-RateLimit-Limit", strconv.FormatFloat(limit, 'f', 0, 64))
			w.Header().Set("X-RateLimit-Remaining", strconv.FormatFloat(remaining, 'f', 0, 64))
			w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(time.Now().Add(time.Second).Unix(), 10))

			if !allowed {
				w.Header().Set("Retry-After", "1")
				reqID := RequestIDFromContext(r.Context())
				types.WriteError(w, http.StatusTooManyRequests, "RATE_LIMIT_EXCEEDED",
					fmt.Sprintf("Rate limit exceeded for %s. Try again in 1 second.", grp), reqID)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
