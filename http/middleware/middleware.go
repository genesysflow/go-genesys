// Package middleware provides built-in middleware for the HTTP layer.
package middleware

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"maps"
	"runtime/debug"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/genesysflow/go-genesys/container"
	"github.com/genesysflow/go-genesys/contracts"
	"github.com/genesysflow/go-genesys/http"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// Recover creates a panic recovery middleware.
func Recover(logger contracts.Logger) http.MiddlewareFunc {
	return func(ctx *http.Context, next func() error) error {
		defer func() {
			if r := recover(); r != nil {
				stack := debug.Stack()
				logger.Error("Panic recovered",
					"error", r,
					"stack", string(stack),
					"path", ctx.Path(),
					"method", ctx.Method(),
				)

				// Try to use error handler
				if app := ctx.App(); app != nil {
					// We use reflection or interface assertion to call Render
					// Since we can't import errors package due to circular dependency (errors imports http)
					// We define a local interface
					type ErrorRenderer interface {
						Render(ctx contracts.Context, err error) error
					}

					if h, err := container.Resolve[any](app, "error.handler"); err == nil {
						if handler, ok := h.(ErrorRenderer); ok {
							err := fmt.Errorf("panic: %v", r)
							_ = handler.Render(ctx, err)
							return
						}
					}
				}

				ctx.Status(fiber.StatusInternalServerError).JSONResponse(fiber.Map{
					"error": "Internal Server Error",
				})
			}
		}()

		return next()
	}
}

// Logger creates a request logging middleware.
func Logger(logger contracts.Logger) http.MiddlewareFunc {
	return func(ctx *http.Context, next func() error) error {
		start := time.Now()

		// Execute request
		err := next()

		// Log request
		duration := time.Since(start)
		logger.Info("HTTP Request",
			"method", ctx.Method(),
			"path", ctx.Path(),
			"status", ctx.FiberCtx().Response().StatusCode(),
			"duration", duration.String(),
			"ip", ctx.IP(),
		)

		return err
	}
}

// RequestID adds a unique request ID to each request.
func RequestID() http.MiddlewareFunc {
	return func(ctx *http.Context, next func() error) error {
		requestID := ctx.Request().Header("X-Request-ID")
		if requestID == "" {
			requestID = uuid.New().String()
		}

		ctx.Set("request_id", requestID)
		ctx.Header("X-Request-ID", requestID)

		return next()
	}
}

// CORS creates a CORS middleware.
func CORS(config ...CORSConfig) http.MiddlewareFunc {
	cfg := DefaultCORSConfig
	if len(config) > 0 {
		cfg = config[0]
	}

	allowedOrigins := splitAndTrim(cfg.AllowOrigins, ",")
	wildcard := cfg.AllowOrigins == "*"

	// The response varies by Origin whenever the allowed origin is derived from
	// the request: either because a specific allowlist is configured, or because
	// credentials force the wildcard to be echoed back as a concrete origin.
	// Only a literal "*" is origin-independent. Without this a shared cache can
	// hand one origin's Access-Control-Allow-Origin — and its
	// Access-Control-Allow-Credentials: true — to a different origin.
	varyOnOrigin := !wildcard || cfg.AllowCredentials

	return func(ctx *http.Context, next func() error) error {
		origin := ctx.Request().Header("Origin")

		if varyOnOrigin {
			ctx.FiberCtx().Vary("Origin")
		}

		allowOrigin := ""
		switch {
		case wildcard && cfg.AllowCredentials:
			// "*" is not a legal Allow-Origin when credentials are permitted;
			// browsers reject the pair outright. Echo the caller's origin so
			// the intent (any origin, with credentials) actually works.
			allowOrigin = origin
		case wildcard:
			allowOrigin = "*"
		default:
			if slices.Contains(allowedOrigins, origin) {
				allowOrigin = origin
			}
		}

		if allowOrigin != "" {
			ctx.Header("Access-Control-Allow-Origin", allowOrigin)
			ctx.Header("Access-Control-Allow-Methods", cfg.AllowMethods)
			ctx.Header("Access-Control-Allow-Headers", cfg.AllowHeaders)

			if cfg.AllowCredentials {
				ctx.Header("Access-Control-Allow-Credentials", "true")
			}

			if cfg.ExposeHeaders != "" {
				ctx.Header("Access-Control-Expose-Headers", cfg.ExposeHeaders)
			}

			if cfg.MaxAge > 0 {
				ctx.Header("Access-Control-Max-Age", strconv.Itoa(cfg.MaxAge))
			}
		}

		// Handle preflight
		if ctx.Method() == "OPTIONS" {
			return ctx.NoContent()
		}

		return next()
	}
}

// CORSConfig defines CORS middleware configuration.
type CORSConfig struct {
	AllowOrigins     string
	AllowMethods     string
	AllowHeaders     string
	AllowCredentials bool
	ExposeHeaders    string
	MaxAge           int
}

// DefaultCORSConfig is the default CORS configuration.
var DefaultCORSConfig = CORSConfig{
	AllowOrigins: "*",
	AllowMethods: "GET,POST,PUT,PATCH,DELETE,OPTIONS",
	AllowHeaders: "Origin,Content-Type,Accept,Authorization",
}

// Timeout bounds how long a request may run by imposing a deadline on the
// request context.
//
// The handler runs on the calling goroutine and the deadline is delivered
// through ctx.Context(), so cancellation is cooperative: database drivers,
// outbound HTTP clients and anything else that honours context.Context will
// abort at the deadline, and the resulting error propagates normally.
//
// It deliberately does NOT run the handler on a separate goroutine and abandon
// it at the deadline. Fiber returns *fiber.Ctx to a pool once the handler
// chain returns, so an abandoned goroutine would keep writing into a context
// that has already been recycled onto another connection — corrupting an
// unrelated client's response and racing on every field of the context. A
// goroutine cannot be killed from the outside in Go, so the only safe design
// is cooperative cancellation.
//
// The corollary is that a handler which ignores its context (a busy loop, or a
// blocking call with no context support) will not be interrupted. Pass the
// context down to make the deadline effective, and keep the kernel's
// ReadTimeout/WriteTimeout as the backstop for a genuinely stuck handler.
func Timeout(timeout time.Duration) http.MiddlewareFunc {
	return func(ctx *http.Context, next func() error) error {
		fiberCtx := ctx.FiberCtx()
		previous := fiberCtx.UserContext()

		timedCtx, cancel := context.WithTimeout(previous, timeout)
		defer cancel()

		fiberCtx.SetUserContext(timedCtx)
		defer fiberCtx.SetUserContext(previous)

		err := next()

		// Report a deadline overrun as 408 rather than surfacing the raw
		// context error, but only if the handler has not already responded.
		if errors.Is(timedCtx.Err(), context.DeadlineExceeded) && err != nil {
			return ctx.Status(fiber.StatusRequestTimeout).JSONResponse(fiber.Map{
				"error": "Request Timeout",
			})
		}

		return err
	}
}

// rateLimiterStore is a concurrency-safe sliding-window request counter.
//
// The map is shared by every in-flight request, so all access must hold the
// mutex: an unsynchronised map here is not merely a lost update, it is a
// "concurrent map read and map write" runtime fatal error that terminates the
// process and cannot be caught by recover().
type rateLimiterStore struct {
	mu       sync.Mutex
	requests map[string][]time.Time
	lastSeen map[string]time.Time
	lastGC   time.Time
}

// gcInterval bounds how often idle keys are swept. Without eviction the map
// grows one entry per distinct source IP and is never reclaimed, which is a
// memory-exhaustion vector on an internet-facing service.
const gcInterval = time.Minute

// allow records a request for key and reports whether it is within the limit.
func (s *rateLimiterStore) allow(key string, now time.Time, maxRequests int, window time.Duration) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.collectIdle(now, window)

	// Drop timestamps that have fallen out of the window.
	valid := s.requests[key][:0]
	for _, t := range s.requests[key] {
		if now.Sub(t) < window {
			valid = append(valid, t)
		}
	}

	if len(valid) >= maxRequests {
		s.requests[key] = valid
		s.lastSeen[key] = now
		return false
	}

	s.requests[key] = append(valid, now)
	s.lastSeen[key] = now
	return true
}

// collectIdle removes keys with no activity for a full window.
// Callers must hold s.mu.
func (s *rateLimiterStore) collectIdle(now time.Time, window time.Duration) {
	if now.Sub(s.lastGC) < gcInterval {
		return
	}
	s.lastGC = now

	for key, seen := range s.lastSeen {
		if now.Sub(seen) >= window {
			delete(s.requests, key)
			delete(s.lastSeen, key)
		}
	}
}

// RateLimiter creates a rate limiting middleware allowing maxRequests per
// window per client IP.
//
// State is per-process and in-memory: behind more than one instance each
// replica enforces its own quota. Use a shared store (Redis) for a global
// limit. The client IP is taken from ctx.IP(), which only reflects
// X-Forwarded-For when trusted proxies are configured on the kernel — see
// http.KernelConfig.TrustedProxies. Without that, a limiter keyed on a
// spoofable header would be trivially bypassed.
func RateLimiter(maxRequests int, window time.Duration) http.MiddlewareFunc {
	store := &rateLimiterStore{
		requests: make(map[string][]time.Time),
		lastSeen: make(map[string]time.Time),
		lastGC:   time.Now(),
	}

	return func(ctx *http.Context, next func() error) error {
		if !store.allow(ctx.IP(), time.Now(), maxRequests, window) {
			ctx.Header("Retry-After", strconv.Itoa(int(window.Seconds())))
			return ctx.Status(fiber.StatusTooManyRequests).JSONResponse(fiber.Map{
				"error": "Too Many Requests",
			})
		}

		return next()
	}
}

// Secure adds security headers.
func Secure(config ...SecureConfig) http.MiddlewareFunc {
	cfg := DefaultSecureConfig
	if len(config) > 0 {
		cfg = config[0]
	}

	return func(ctx *http.Context, next func() error) error {
		if cfg.XSSProtection != "" {
			ctx.Header("X-XSS-Protection", cfg.XSSProtection)
		}
		if cfg.ContentTypeNosniff != "" {
			ctx.Header("X-Content-Type-Options", cfg.ContentTypeNosniff)
		}
		if cfg.XFrameOptions != "" {
			ctx.Header("X-Frame-Options", cfg.XFrameOptions)
		}
		if cfg.HSTSMaxAge > 0 {
			// strconv, not string(rune(n)): the latter yields the Unicode
			// character with that code point, producing a malformed header
			// that browsers discard — HSTS would appear configured while
			// silently never applying.
			hsts := "max-age=" + strconv.Itoa(cfg.HSTSMaxAge)
			if cfg.HSTSIncludeSubdomains {
				hsts += "; includeSubDomains"
			}
			if cfg.HSTSPreload {
				hsts += "; preload"
			}
			ctx.Header("Strict-Transport-Security", hsts)
		}
		if cfg.ContentSecurityPolicy != "" {
			ctx.Header("Content-Security-Policy", cfg.ContentSecurityPolicy)
		}
		if cfg.ReferrerPolicy != "" {
			ctx.Header("Referrer-Policy", cfg.ReferrerPolicy)
		}

		return next()
	}
}

// SecureConfig defines security headers configuration.
type SecureConfig struct {
	XSSProtection         string
	ContentTypeNosniff    string
	XFrameOptions         string
	HSTSMaxAge            int
	HSTSIncludeSubdomains bool
	HSTSPreload           bool
	ContentSecurityPolicy string
	ReferrerPolicy        string
}

// DefaultSecureConfig is the default security configuration.
//
// HSTSMaxAge is 0 (header omitted) because enabling HSTS on a host that is not
// yet fully served over TLS locks clients out of it for the lifetime of the
// max-age. Set it explicitly — 31536000 with IncludeSubdomains is the usual
// production value — once HTTPS is known to work on every subdomain.
//
// ContentSecurityPolicy is likewise empty: an effective policy is
// application-specific, and a wrong default would either break pages or give
// the illusion of protection.
//
// XSSProtection is "0", which disables the legacy XSS auditor. Every current
// browser has removed that auditor, and in the browsers that did ship it the
// filter was itself exploitable to introduce cross-site leaks — "1;
// mode=block" is actively discouraged today. Use ContentSecurityPolicy for
// real XSS defence.
var DefaultSecureConfig = SecureConfig{
	XSSProtection:      "0",
	ContentTypeNosniff: "nosniff",
	XFrameOptions:      "SAMEORIGIN",
	ReferrerPolicy:     "strict-origin-when-cross-origin",
}

// Compress creates a compression middleware.
// Note: Fiber has built-in compression, this is for custom handling.
func Compress() http.MiddlewareFunc {
	return func(ctx *http.Context, next func() error) error {
		// Let Fiber handle compression
		return next()
	}
}

// BasicAuth creates an HTTP Basic authentication middleware.
//
// users maps username to password. Credentials are compared in constant time
// so that neither the username nor the password can be recovered by timing the
// response. Note that Basic auth transmits the password on every request, so
// this must only be served over TLS.
func BasicAuth(users map[string]string) http.MiddlewareFunc {
	// Copy the credentials so later mutation of the caller's map cannot
	// silently change who is authorised.
	credentials := make(map[string]string, len(users))
	maps.Copy(credentials, users)

	return func(ctx *http.Context, next func() error) error {
		username, password, ok := ctx.Request().BasicAuth()
		if !ok {
			return basicAuthChallenge(ctx)
		}

		expected, found := credentials[username]

		// Always run the comparison, even for an unknown username, so that a
		// valid username is not distinguishable by response time.
		if !found {
			expected = ""
		}
		match := subtle.ConstantTimeCompare([]byte(password), []byte(expected)) == 1

		if !found || !match {
			return basicAuthChallenge(ctx)
		}

		ctx.Set("auth.user", username)

		return next()
	}
}

// basicAuthChallenge rejects the request with a WWW-Authenticate challenge.
func basicAuthChallenge(ctx *http.Context) error {
	ctx.Header("WWW-Authenticate", `Basic realm="Restricted", charset="UTF-8"`)
	return ctx.Unauthorized()
}

// splitAndTrim splits a string and trims whitespace.
func splitAndTrim(s, sep string) []string {
	var result []string
	for _, part := range split(s, sep) {
		trimmed := trim(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func split(s, sep string) []string {
	if s == "" {
		return nil
	}
	var result []string
	start := 0
	for i := 0; i <= len(s)-len(sep); i++ {
		if s[i:i+len(sep)] == sep {
			result = append(result, s[start:i])
			start = i + len(sep)
		}
	}
	result = append(result, s[start:])
	return result
}

func trim(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}
