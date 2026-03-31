package middleware

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/garudax-platform/gateway/internal/types"
	"github.com/redis/go-redis/v9"
)

// RedisRateLimiter implements sliding-window rate limiting backed by Redis.
// Each window uses a Redis key with INCR + EXPIRE for atomic counting.
// If Redis becomes unreachable, it falls back to the in-memory RateLimitGroup.
type RedisRateLimiter struct {
	rdb      *redis.Client
	fallback *RateLimitGroup
	window   time.Duration
}

// NewRedisRateLimiter creates a Redis-backed rate limiter.  If redisURL
// is empty or Redis is unreachable, it returns nil and the caller should
// use the in-memory fallback.
func NewRedisRateLimiter(redisURL string, fallback *RateLimitGroup, window time.Duration) (*RedisRateLimiter, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parse redis url: %w", err)
	}

	rdb := redis.NewClient(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		rdb.Close()
		return nil, fmt.Errorf("redis ping: %w", err)
	}

	if window <= 0 {
		window = time.Minute
	}

	return &RedisRateLimiter{
		rdb:      rdb,
		fallback: fallback,
		window:   window,
	}, nil
}

// NewRedisRateLimiterFromClient creates a RedisRateLimiter from an existing
// redis.Client (useful for testing).
func NewRedisRateLimiterFromClient(rdb *redis.Client, fallback *RateLimitGroup, window time.Duration) *RedisRateLimiter {
	if window <= 0 {
		window = time.Minute
	}
	return &RedisRateLimiter{
		rdb:      rdb,
		fallback: fallback,
		window:   window,
	}
}

// allow checks whether a request should be allowed for the given group and
// key.  Returns (allowed, current_count, limit).
func (rl *RedisRateLimiter) allow(group, key string, cfg *RateLimitConfig) (bool, int64, int64) {
	ctx := context.Background()

	// Compute the limit for one window period.
	// The existing RateLimitConfig uses Rate (tokens/sec) and Burst.
	// For the sliding window we use: limit = Rate * windowSeconds.
	// This keeps the same throughput semantics as the token-bucket approach.
	windowSecs := int64(rl.window.Seconds())
	if windowSecs < 1 {
		windowSecs = 60
	}
	limit := int64(cfg.Rate * float64(windowSecs))
	if limit < 1 {
		limit = 1
	}

	// Window key includes a time-bucket so keys rotate automatically.
	bucket := time.Now().Unix() / windowSecs
	redisKey := fmt.Sprintf("ratelimit:%s:%s:%d", group, key, bucket)

	// INCR + EXPIRE is atomic enough for rate limiting purposes.
	count, err := rl.rdb.Incr(ctx, redisKey).Result()
	if err != nil {
		// Redis error — fall back to in-memory limiter.
		log.Printf("[WARN] redis rate limiter INCR failed: %v — falling back to in-memory", err)
		allowed, remaining, max := rl.fallback.GetLimiter(group).Allow(key)
		return allowed, int64(remaining), int64(max)
	}

	// Set TTL on first increment so keys expire automatically.
	if count == 1 {
		rl.rdb.Expire(ctx, redisKey, rl.window+time.Second)
	}

	allowed := count <= limit
	return allowed, count, limit
}

// Close closes the Redis connection.
func (rl *RedisRateLimiter) Close() error {
	return rl.rdb.Close()
}

// RedisRateLimit creates rate limiting middleware backed by Redis.
// Falls back to in-memory if Redis is unreachable per-request.
func RedisRateLimit(rl *RedisRateLimiter, groupFn func(*http.Request) string, keyFn func(*http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			grp := groupFn(r)
			key := keyFn(r)

			// Look up the config for this group from the fallback (which
			// holds the canonical rate/burst configs).
			limiter := rl.fallback.GetLimiter(grp)
			cfg := limiter.config

			allowed, count, limit := rl.allow(grp, key, cfg)

			remaining := limit - count
			if remaining < 0 {
				remaining = 0
			}

			w.Header().Set("X-RateLimit-Limit", strconv.FormatInt(limit, 10))
			w.Header().Set("X-RateLimit-Remaining", strconv.FormatInt(remaining, 10))
			w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(time.Now().Add(rl.window).Unix(), 10))

			if !allowed {
				retryAfter := int(rl.window.Seconds())
				if retryAfter < 1 {
					retryAfter = 1
				}
				w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
				reqID := RequestIDFromContext(r.Context())
				types.WriteError(w, http.StatusTooManyRequests, "RATE_LIMIT_EXCEEDED",
					fmt.Sprintf("Rate limit exceeded for %s. Try again in %d seconds.", grp, retryAfter), reqID)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
