package http

import (
	"net/http/httptest"
	"testing"

	"github.com/genesysflow/go-genesys/contracts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubAuthorizer answers from a fixed set of allowed abilities and
// records what it was asked, so argument passing can be checked.
type stubAuthorizer struct {
	allowed map[string]bool
	asked   []string
	lastArg any
}

func (s *stubAuthorizer) Allows(ability string, args ...any) bool {
	s.asked = append(s.asked, ability)
	if len(args) > 0 {
		s.lastArg = args[0]
	}
	return s.allowed[ability]
}

// With no gate bound the context still answers, and it answers no: a
// forgotten middleware must fail closed rather than authorize everything.
func TestContextGateDefaultsToDenyAll(t *testing.T) {
	runHandler(t, func(ctx *Context) error {
		gate := ctx.Gate()
		require.NotNil(t, gate)
		assert.IsType(t, denyAll{}, gate)

		assert.False(t, gate.Allows("update"))
		assert.False(t, gate.Allows("update", "any", "args"))
		assert.False(t, ctx.Can("update"))
		assert.True(t, ctx.Cannot("update"))

		return ctx.String("ok")
	})
}

// A value stored under the gate key that is not an Authorizer is
// ignored, so the fallback still denies.
func TestContextGateIgnoresWrongType(t *testing.T) {
	runHandler(t, func(ctx *Context) error {
		ctx.Set(contextGateKey, "not an authorizer")
		assert.IsType(t, denyAll{}, ctx.Gate())
		assert.False(t, ctx.Can("update"))
		return ctx.String("ok")
	})
}

// A nil Authorizer stored explicitly is treated as no gate at all.
func TestContextGateIgnoresNilAuthorizer(t *testing.T) {
	runHandler(t, func(ctx *Context) error {
		ctx.SetGate(nil)
		assert.IsType(t, denyAll{}, ctx.Gate())
		return ctx.String("ok")
	})
}

func TestContextSetGateAndCan(t *testing.T) {
	gate := &stubAuthorizer{allowed: map[string]bool{"update": true}}
	post := struct{ ID int }{ID: 7}

	runHandler(t, func(ctx *Context) error {
		ctx.SetGate(gate)

		assert.Same(t, gate, ctx.Gate())
		assert.True(t, ctx.Can("update", post))
		assert.False(t, ctx.Cannot("update", post))
		assert.False(t, ctx.Can("delete", post))
		assert.True(t, ctx.Cannot("delete", post))

		return ctx.String("ok")
	})

	assert.Equal(t, []string{"update", "update", "delete", "delete"}, gate.asked)
	assert.Equal(t, post, gate.lastArg, "the subject reaches the authorizer")
}

// Authorize is the one-line guard: nil when allowed.
func TestContextAuthorizeAllows(t *testing.T) {
	gate := &stubAuthorizer{allowed: map[string]bool{"update": true}}

	resp, body := serveContext(t, "GET", "/", httptest.NewRequest("GET", "/", nil), func(ctx *Context) error {
		ctx.SetGate(gate)
		if err := ctx.Authorize("update"); err != nil {
			return err
		}
		return ctx.String("allowed")
	})

	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "allowed", body)
}

// Denied, it returns a 403 HTTP error the error handler can render.
func TestContextAuthorizeDenies(t *testing.T) {
	gate := &stubAuthorizer{allowed: map[string]bool{}}

	var returned error
	runHandler(t, func(ctx *Context) error {
		ctx.SetGate(gate)
		returned = ctx.Authorize("update", "the post")
		return ctx.String("ok")
	})

	require.Error(t, returned)

	httpErr, ok := returned.(contracts.HTTPError)
	require.True(t, ok, "Authorize should return an HTTPError, got %T", returned)
	assert.Equal(t, 403, httpErr.StatusCode())
	assert.Equal(t, "This action is unauthorized.", httpErr.Message())
	assert.Equal(t, "the post", gate.lastArg)
}

// With no gate bound at all, Authorize denies.
func TestContextAuthorizeWithoutGate(t *testing.T) {
	var returned error
	runHandler(t, func(ctx *Context) error {
		returned = ctx.Authorize("update")
		return ctx.String("ok")
	})

	require.Error(t, returned)
	assert.Equal(t, 403, returned.(contracts.HTTPError).StatusCode())
}
