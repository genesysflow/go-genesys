package middleware

import (
	"fmt"
	"time"

	"github.com/genesysflow/go-genesys/cache"
	"github.com/genesysflow/go-genesys/http"
)

// ThrottleConfig configures a named, cache-backed rate limiter -
// Laravel's throttle:60,1 with a shared store, so limits hold across
// restarts and across instances when the store is Redis.
type ThrottleConfig struct {
	// Store persists the counters. Required.
	Store cache.Store

	// Name namespaces the limiter ("api", "login", ...). Default "global".
	Name string

	// MaxRequests allowed per window. Default 60.
	MaxRequests int

	// Window duration. Default one minute.
	Window time.Duration

	// KeyFor derives the per-client key. Defaults to the client IP, so
	// each IP gets its own allowance.
	KeyFor func(ctx *http.Context) string
}

// Throttle rate-limits requests using a cache store. Responses carry
// X-RateLimit-Limit / X-RateLimit-Remaining, and rejected requests get
// a 429 with a Retry-After header.
func Throttle(config ThrottleConfig) http.MiddlewareFunc {
	if config.Name == "" {
		config.Name = "global"
	}
	if config.MaxRequests <= 0 {
		config.MaxRequests = 60
	}
	if config.Window <= 0 {
		config.Window = time.Minute
	}
	if config.KeyFor == nil {
		config.KeyFor = func(ctx *http.Context) string { return ctx.IP() }
	}

	return func(ctx *http.Context, next func() error) error {
		key := fmt.Sprintf("throttle:%s:%s", config.Name, config.KeyFor(ctx))

		// Add seeds the counter with the window's TTL; Increment then
		// counts this request. When the key expires the window resets.
		if _, err := config.Store.Add(key, 0, config.Window); err != nil {
			return next() // a broken store should not take the site down
		}
		hits, err := config.Store.Increment(key, 1)
		if err != nil {
			return next()
		}

		remaining := int64(config.MaxRequests) - hits
		if remaining < 0 {
			remaining = 0
		}
		ctx.Header("X-RateLimit-Limit", fmt.Sprintf("%d", config.MaxRequests))
		ctx.Header("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))

		if hits > int64(config.MaxRequests) {
			retryAfter := int(config.Window.Seconds())
			ctx.Header("Retry-After", fmt.Sprintf("%d", retryAfter))
			return ctx.AbortWithJSON(429, map[string]any{
				"message": "Too Many Requests",
			})
		}
		return next()
	}
}
