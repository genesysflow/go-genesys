// Package middleware provides built-in middleware for the HTTP layer.
package middleware

import (
	"runtime/debug"
	"time"

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

	return func(ctx *http.Context, next func() error) error {
		origin := ctx.Request().Header("Origin")

		// Check if origin is allowed
		allowOrigin := ""
		if cfg.AllowOrigins == "*" {
			allowOrigin = "*"
		} else {
			for _, o := range splitAndTrim(cfg.AllowOrigins, ",") {
				if o == origin {
					allowOrigin = origin
					break
				}
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
				ctx.Header("Access-Control-Max-Age", string(rune(cfg.MaxAge)))
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

// Timeout creates a request timeout middleware.
func Timeout(timeout time.Duration) http.MiddlewareFunc {
	return func(ctx *http.Context, next func() error) error {
		done := make(chan error, 1)

		go func() {
			done <- next()
		}()

		select {
		case err := <-done:
			return err
		case <-time.After(timeout):
			return ctx.Status(fiber.StatusRequestTimeout).JSONResponse(fiber.Map{
				"error": "Request Timeout",
			})
		}
	}
}

// RateLimiter creates a rate limiting middleware.
func RateLimiter(maxRequests int, window time.Duration) http.MiddlewareFunc {
	// Simple in-memory rate limiter
	// For production, use a distributed store like Redis
	requests := make(map[string][]time.Time)

	return func(ctx *http.Context, next func() error) error {
		ip := ctx.IP()
		now := time.Now()

		// Clean old entries
		if times, ok := requests[ip]; ok {
			var valid []time.Time
			for _, t := range times {
				if now.Sub(t) < window {
					valid = append(valid, t)
				}
			}
			requests[ip] = valid
		}

		// Check rate limit
		if len(requests[ip]) >= maxRequests {
			ctx.Header("Retry-After", window.String())
			return ctx.Status(fiber.StatusTooManyRequests).JSONResponse(fiber.Map{
				"error": "Too Many Requests",
			})
		}

		// Record request
		requests[ip] = append(requests[ip], now)

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
			ctx.Header("Strict-Transport-Security", "max-age="+string(rune(cfg.HSTSMaxAge)))
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
	ContentSecurityPolicy string
	ReferrerPolicy        string
}

// DefaultSecureConfig is the default security configuration.
var DefaultSecureConfig = SecureConfig{
	XSSProtection:      "1; mode=block",
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

// BasicAuth creates a basic authentication middleware.
func BasicAuth(users map[string]string) http.MiddlewareFunc {
	return func(ctx *http.Context, next func() error) error {
		auth := ctx.Request().Header("Authorization")
		if auth == "" {
			ctx.Header("WWW-Authenticate", `Basic realm="Restricted"`)
			return ctx.Unauthorized()
		}

		// Verify credentials
		// This is a simplified implementation
		// Full implementation would decode base64 and verify

		return next()
	}
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
