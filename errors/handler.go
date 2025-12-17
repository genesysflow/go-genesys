// Package errors provides error handling and panic recovery.
package errors

import (
	"fmt"
	"net/http"
	"runtime/debug"

	"github.com/genesysflow/go-genesys/contracts"
	"github.com/gofiber/fiber/v2"
)

// Handler handles errors and panics.
type Handler struct {
	logger     contracts.Logger
	debug      bool
	dontReport []error
	reporters  []Reporter
}

// Reporter is a function that reports errors.
type Reporter func(err error, ctx contracts.Context)

// Config holds error handler configuration.
type Config struct {
	Debug      bool
	Logger     contracts.Logger
	DontReport []error
}

// NewHandler creates a new error handler.
func NewHandler(config ...Config) *Handler {
	h := &Handler{
		dontReport: make([]error, 0),
		reporters:  make([]Reporter, 0),
	}

	if len(config) > 0 {
		h.debug = config[0].Debug
		h.logger = config[0].Logger
		h.dontReport = config[0].DontReport
	}

	return h
}

// SetLogger sets the logger.
func (h *Handler) SetLogger(logger contracts.Logger) {
	h.logger = logger
}

// SetDebug sets debug mode.
func (h *Handler) SetDebug(debug bool) {
	h.debug = debug
}

// AddReporter adds an error reporter.
func (h *Handler) AddReporter(reporter Reporter) {
	h.reporters = append(h.reporters, reporter)
}

// DontReport adds errors that should not be reported.
func (h *Handler) DontReport(errs ...error) {
	h.dontReport = append(h.dontReport, errs...)
}

// Handle handles an error.
func (h *Handler) Handle(ctx contracts.Context, err error) error {
	if h.ShouldReport(err) {
		h.Report(err, ctx)
	}
	return h.Render(ctx, err)
}

// Report reports an error for logging.
func (h *Handler) Report(err error, ctx ...contracts.Context) {
	if h.logger != nil {
		fields := map[string]any{
			"error": err.Error(),
		}

		if len(ctx) > 0 && ctx[0] != nil {
			fields["path"] = ctx[0].Request().Path()
			fields["method"] = ctx[0].Request().Method()
			fields["ip"] = ctx[0].Request().IP()
		}

		h.logger.WithFields(fields).Error("Error occurred")
	}

	// Call custom reporters
	for _, reporter := range h.reporters {
		if len(ctx) > 0 {
			reporter(err, ctx[0])
		} else {
			reporter(err, nil)
		}
	}
}

// ShouldReport determines if the error should be reported.
func (h *Handler) ShouldReport(err error) bool {
	for _, dontReport := range h.dontReport {
		if err == dontReport {
			return false
		}
	}
	return true
}

// Render renders an error response.
func (h *Handler) Render(ctx contracts.Context, err error) error {
	code := http.StatusInternalServerError
	message := "Internal Server Error"

	// Check for HTTP error
	if httpErr, ok := err.(contracts.HTTPError); ok {
		code = httpErr.StatusCode()
		message = httpErr.Message()
	}

	// Check for Fiber error
	if fiberErr, ok := err.(*fiber.Error); ok {
		code = fiberErr.Code
		message = fiberErr.Message
	}

	// Build response
	response := map[string]any{
		"success": false,
		"error":   message,
		"status":  code,
	}

	// Include stack trace in debug mode
	if h.debug {
		response["exception"] = err.Error()
		response["stack"] = string(debug.Stack())
	}

	return ctx.Status(code).JSONResponse(response)
}

// RecoverMiddleware creates a panic recovery middleware.
func (h *Handler) RecoverMiddleware() contracts.MiddlewareFunc {
	return func(ctx contracts.Context, next func() error) error {
		defer func() {
			if r := recover(); r != nil {
				var err error
				switch v := r.(type) {
				case error:
					err = v
				case string:
					err = fmt.Errorf("%s", v)
				default:
					err = fmt.Errorf("%v", v)
				}

				// Log the panic
				if h.logger != nil {
					h.logger.Error("Panic recovered",
						"error", err.Error(),
						"stack", string(debug.Stack()),
						"path", ctx.Request().Path(),
						"method", ctx.Request().Method(),
					)
				}

				// Render error response
				h.Render(ctx, contracts.NewHTTPError(
					http.StatusInternalServerError,
					"Internal Server Error",
					err,
				))
			}
		}()

		return next()
	}
}

// HTTPError is a convenience function to create HTTP errors.
func HTTPError(code int, message string, err ...error) contracts.HTTPError {
	return contracts.NewHTTPError(code, message, err...)
}

// BadRequest creates a 400 Bad Request error.
func BadRequest(message string, err ...error) contracts.HTTPError {
	return contracts.NewHTTPError(http.StatusBadRequest, message, err...)
}

// Unauthorized creates a 401 Unauthorized error.
func Unauthorized(message ...string) contracts.HTTPError {
	msg := "Unauthorized"
	if len(message) > 0 {
		msg = message[0]
	}
	return contracts.NewHTTPError(http.StatusUnauthorized, msg)
}

// Forbidden creates a 403 Forbidden error.
func Forbidden(message ...string) contracts.HTTPError {
	msg := "Forbidden"
	if len(message) > 0 {
		msg = message[0]
	}
	return contracts.NewHTTPError(http.StatusForbidden, msg)
}

// NotFound creates a 404 Not Found error.
func NotFound(message ...string) contracts.HTTPError {
	msg := "Not Found"
	if len(message) > 0 {
		msg = message[0]
	}
	return contracts.NewHTTPError(http.StatusNotFound, msg)
}

// MethodNotAllowed creates a 405 Method Not Allowed error.
func MethodNotAllowed(message ...string) contracts.HTTPError {
	msg := "Method Not Allowed"
	if len(message) > 0 {
		msg = message[0]
	}
	return contracts.NewHTTPError(http.StatusMethodNotAllowed, msg)
}

// Conflict creates a 409 Conflict error.
func Conflict(message string, err ...error) contracts.HTTPError {
	return contracts.NewHTTPError(http.StatusConflict, message, err...)
}

// UnprocessableEntity creates a 422 Unprocessable Entity error.
func UnprocessableEntity(message string, err ...error) contracts.HTTPError {
	return contracts.NewHTTPError(http.StatusUnprocessableEntity, message, err...)
}

// TooManyRequests creates a 429 Too Many Requests error.
func TooManyRequests(message ...string) contracts.HTTPError {
	msg := "Too Many Requests"
	if len(message) > 0 {
		msg = message[0]
	}
	return contracts.NewHTTPError(http.StatusTooManyRequests, msg)
}

// InternalServerError creates a 500 Internal Server Error error.
func InternalServerError(message string, err ...error) contracts.HTTPError {
	return contracts.NewHTTPError(http.StatusInternalServerError, message, err...)
}

// ServiceUnavailable creates a 503 Service Unavailable error.
func ServiceUnavailable(message ...string) contracts.HTTPError {
	msg := "Service Unavailable"
	if len(message) > 0 {
		msg = message[0]
	}
	return contracts.NewHTTPError(http.StatusServiceUnavailable, msg)
}

// ValidationError represents a validation error with field errors.
type ValidationError struct {
	Message string              `json:"message"`
	Errors  map[string][]string `json:"errors"`
}

// NewValidationError creates a new validation error.
func NewValidationError(errors map[string][]string) *ValidationError {
	return &ValidationError{
		Message: "Validation failed",
		Errors:  errors,
	}
}

// Error implements the error interface.
func (e *ValidationError) Error() string {
	return e.Message
}

// StatusCode returns the HTTP status code.
func (e *ValidationError) StatusCode() int {
	return http.StatusUnprocessableEntity
}
