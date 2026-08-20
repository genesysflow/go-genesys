package errors_test

import (
	"fmt"
	"testing"

	goerrors "errors"

	"github.com/genesysflow/go-genesys/contracts"
	frameworkerrors "github.com/genesysflow/go-genesys/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTTPErrorConstructors(t *testing.T) {
	cases := []struct {
		err      interface{ StatusCode() int }
		expected int
	}{
		{frameworkerrors.BadRequest("x"), 400},
		{frameworkerrors.Unauthorized(), 401},
		{frameworkerrors.Forbidden(), 403},
		{frameworkerrors.NotFound(), 404},
		{frameworkerrors.MethodNotAllowed(), 405},
		{frameworkerrors.Conflict("x"), 409},
		{frameworkerrors.UnprocessableEntity("x"), 422},
		{frameworkerrors.TooManyRequests(), 429},
		{frameworkerrors.InternalServerError("x"), 500},
		{frameworkerrors.ServiceUnavailable(), 503},
		{frameworkerrors.HTTPError(418, "teapot"), 418},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.expected, tc.err.StatusCode())
	}
}

func TestWrappedHTTPErrorsKeepStatus(t *testing.T) {
	wrapped := fmt.Errorf("loading user: %w", frameworkerrors.NotFound("User missing"))

	var httpErr interface{ StatusCode() int }
	require.True(t, goerrors.As(wrapped, &httpErr))
	assert.Equal(t, 404, httpErr.StatusCode())
}

func TestWithStackCapturesOnce(t *testing.T) {
	assert.Nil(t, frameworkerrors.WithStack(nil))

	base := goerrors.New("boom")
	traced := frameworkerrors.WithStack(base)

	var withStack interface{ Stack() string }
	require.True(t, goerrors.As(traced, &withStack))
	assert.Contains(t, withStack.Stack(), "goroutine")
	assert.ErrorIs(t, traced, base)

	// Re-wrapping keeps the original capture instead of re-capturing.
	again := frameworkerrors.WithStack(traced)
	assert.Same(t, traced, again)
}

func TestValidationErrorShape(t *testing.T) {
	verr := frameworkerrors.NewValidationError(map[string][]string{
		"email": {"Email is required"},
	})
	assert.Equal(t, "The given data was invalid.", verr.Error())
	assert.Equal(t, 422, verr.StatusCode())
	assert.Equal(t, []string{"Email is required"}, verr.Errors["email"])
}

func TestReportersAndDontReport(t *testing.T) {
	handler := frameworkerrors.NewHandler()

	var reported []error
	handler.AddReporter(func(err error, _ contracts.Context) {
		reported = append(reported, err)
	})

	boom := goerrors.New("boom")
	handler.Report(boom)
	require.Len(t, reported, 1)
	assert.ErrorIs(t, reported[0], boom)

	ignored := goerrors.New("ignore me")
	handler.DontReport(ignored)
	assert.False(t, handler.ShouldReport(ignored))
	assert.True(t, handler.ShouldReport(goerrors.New("report me")))
}
