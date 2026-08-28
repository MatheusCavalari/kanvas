package middleware

import (
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// NewRateLimiter returns chi-style middleware implementing a sliding
// window rate limiter backed by Redis. It approximates a sliding window
// by combining the request count from the previous fixed window (weighted
// by how much of it overlaps the sliding window) with the count from the
// current fixed window.
//
// keyFn extracts a rate-limit key from the request (e.g. by IP or user
// ID). If keyFn returns an empty string, the request is not rate limited
// (this lets the same middleware be reused for e.g. write-only or
// read-only limits, where keyFn returns "" for requests that don't
// match).
func NewRateLimiter(client *redis.Client, limit int, window time.Duration, keyFn func(*http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := keyFn(r)
			if key == "" {
				next.ServeHTTP(w, r)
				return
			}

			now := time.Now()
			windowSec := int64(window.Seconds())
			currentWindow := now.Unix() / windowSec
			elapsed := float64(now.Unix() % windowSec)
			weight := 1.0 - (elapsed / float64(windowSec))

			prevKey := fmt.Sprintf("rl:%s:%d", key, currentWindow-1)
			currKey := fmt.Sprintf("rl:%s:%d", key, currentWindow)

			ctx := r.Context()
			pipe := client.Pipeline()
			prevCmd := pipe.Get(ctx, prevKey)
			incrCmd := pipe.Incr(ctx, currKey)
			pipe.Expire(ctx, currKey, window+time.Second)
			_, err := pipe.Exec(ctx)
			if err != nil && err != redis.Nil {
				// Redis is unavailable or the pipeline failed for some
				// other reason. Fail open rather than blocking all
				// traffic on a rate limiter outage.
				next.ServeHTTP(w, r)
				return
			}

			prevCount, _ := strconv.ParseFloat(prevCmd.Val(), 64)
			currCount := float64(incrCmd.Val())
			effective := prevCount*weight + currCount

			remaining := limit - int(effective)
			if remaining < 0 {
				remaining = 0
			}
			resetAt := (currentWindow + 1) * windowSec

			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(limit))
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
			w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(resetAt, 10))

			if effective > float64(limit) {
				retryAfter := resetAt - now.Unix()
				w.Header().Set("Retry-After", strconv.FormatInt(retryAfter, 10))
				writeError(w, http.StatusTooManyRequests, "rate_limit_exceeded", "too many requests")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// IPKey returns a rate-limit key derived from the request's remote IP
// address, for use as a keyFn with NewRateLimiter.
func IPKey(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return "ip:" + r.RemoteAddr
	}
	return "ip:" + host
}

// UserKey returns a rate-limit key derived from the authenticated user ID
// stored in the request context by the Auth middleware. It returns "" if
// no user is present, which causes NewRateLimiter to skip rate limiting
// for that request (e.g. unauthenticated requests, which should already
// be rejected by Auth running earlier in the chain).
func UserKey(r *http.Request) string {
	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		return ""
	}
	return "user:" + userID.String()
}

// UserWriteKey behaves like UserKey but only rate limits state-changing
// requests (anything other than GET, HEAD, or OPTIONS).
func UserWriteKey(r *http.Request) string {
	if r.Method == http.MethodGet || r.Method == http.MethodOptions || r.Method == http.MethodHead {
		return ""
	}
	return UserKey(r)
}

// UserReadKey behaves like UserKey but only rate limits GET requests.
func UserReadKey(r *http.Request) string {
	if r.Method != http.MethodGet {
		return ""
	}
	return UserKey(r)
}
