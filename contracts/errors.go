package contracts

import "net/http"

// ErrorHandler defines the interface for error handling.
type ErrorHandler interface {
	// Handle handles an error.
	Handle(ctx Context, err error) error

	// Report reports an error for logging.
	Report(err error)

	// ShouldReport determines if the error should be reported.
	ShouldReport(err error) bool

	// Render renders an error response.
	Render(ctx Context, err error) error
}

// HTTPError represents an HTTP error with status code.
type HTTPError interface {
	error

	// StatusCode returns the HTTP status code.
	StatusCode() int

	// Message returns the error message.
	Message() string

	// Unwrap returns the underlying error.
	Unwrap() error
}

// httpError is the default implementation of HTTPError.
type httpError struct {
	code    int
	message string
	err     error
}

// NewHTTPError creates a new HTTP error.
func NewHTTPError(code int, message string, err ...error) HTTPError {
	e := &httpError{
		code:    code,
		message: message,
	}
	if len(err) > 0 {
		e.err = err[0]
	}
	return e
}

func (e *httpError) Error() string {
	if e.err != nil {
		return e.err.Error()
	}
	return e.message
}

func (e *httpError) StatusCode() int {
	return e.code
}

func (e *httpError) Message() string {
	return e.message
}

func (e *httpError) Unwrap() error {
	return e.err
}

// Common HTTP errors.
var (
	ErrBadRequest          = NewHTTPError(http.StatusBadRequest, "Bad Request")
	ErrUnauthorized        = NewHTTPError(http.StatusUnauthorized, "Unauthorized")
	ErrForbidden           = NewHTTPError(http.StatusForbidden, "Forbidden")
	ErrNotFound            = NewHTTPError(http.StatusNotFound, "Not Found")
	ErrMethodNotAllowed    = NewHTTPError(http.StatusMethodNotAllowed, "Method Not Allowed")
	ErrConflict            = NewHTTPError(http.StatusConflict, "Conflict")
	ErrUnprocessableEntity = NewHTTPError(http.StatusUnprocessableEntity, "Unprocessable Entity")
	ErrTooManyRequests     = NewHTTPError(http.StatusTooManyRequests, "Too Many Requests")
	ErrInternalServer      = NewHTTPError(http.StatusInternalServerError, "Internal Server Error")
	ErrServiceUnavailable  = NewHTTPError(http.StatusServiceUnavailable, "Service Unavailable")
)
