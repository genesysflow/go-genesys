package auth_test

import (
	"net/http/httptest"
	"testing"

	"github.com/genesysflow/go-genesys/auth"
	"github.com/genesysflow/go-genesys/database"
	genhttp "github.com/genesysflow/go-genesys/http"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// gateKernel wires a gate with the Post policy and an authenticated user.
func gateKernel(t *testing.T, userID int64) (*genhttp.Kernel, *auth.Gate) {
	t.Helper()

	kernel, _, _ := setupAuthApp(t)
	gate := auth.NewGate()
	require.NoError(t, auth.RegisterPolicy[Post](gate, &PostPolicy{}))

	kernel.Use(func(ctx *genhttp.Context, next func() error) error {
		if userID > 0 {
			ctx.SetUser(&User{Model: database.Model{ID: userID}})
		}
		return next()
	})
	kernel.Use(auth.GateMiddleware(gate))

	return kernel, gate
}

func TestContextCanUsesTheBoundGate(t *testing.T) {
	kernel, _ := gateKernel(t, 1)

	kernel.GET("/can", func(ctx *genhttp.Context) error {
		mine := &Post{AuthorID: 1}
		theirs := &Post{AuthorID: 2}

		assert.True(t, ctx.Can("update", mine))
		assert.False(t, ctx.Can("update", theirs))
		assert.True(t, ctx.Cannot("update", theirs))
		return ctx.String("ok")
	})

	resp, err := kernel.Fiber().Test(httptest.NewRequest("GET", "/can", nil), -1)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

// With no gate bound, authorization fails closed: a forgotten middleware
// must not authorize by accident.
func TestContextCanDeniesWithoutGate(t *testing.T) {
	kernel, _, _ := setupAuthApp(t)

	kernel.GET("/no-gate", func(ctx *genhttp.Context) error {
		assert.False(t, ctx.Can("update", &Post{AuthorID: 1}))
		return ctx.String("ok")
	})

	resp, err := kernel.Fiber().Test(httptest.NewRequest("GET", "/no-gate", nil), -1)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestContextAuthorizeReturns403(t *testing.T) {
	kernel, _ := gateKernel(t, 2)

	kernel.GET("/authorize", func(ctx *genhttp.Context) error {
		return ctx.Authorize("update", &Post{AuthorID: 1})
	})

	resp, err := kernel.Fiber().Test(httptest.NewRequest("GET", "/authorize", nil), -1)
	require.NoError(t, err)
	assert.Equal(t, 403, resp.StatusCode)
}

// The gate reaches views, so a template can hide what the user may not do.
func TestGateIsSharedWithViews(t *testing.T) {
	kernel, _ := gateKernel(t, 1)

	kernel.GET("/view-data", func(ctx *genhttp.Context) error {
		gate, ok := ctx.ViewData()["gate"].(genhttp.Authorizer)
		require.True(t, ok, "views should receive the gate")
		assert.True(t, gate.Allows("update", &Post{AuthorID: 1}))
		return ctx.String("ok")
	})

	resp, err := kernel.Fiber().Test(httptest.NewRequest("GET", "/view-data", nil), -1)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

// UserPolicy lets a user act only on their own record.
type UserPolicy struct{}

func (p *UserPolicy) Update(actor auth.Authenticatable, target *User) bool {
	return actor != nil && actor.GetAuthIdentifier() == target.ID
}

func (p *UserPolicy) Create(actor auth.Authenticatable) bool {
	return actor != nil
}

// The can middleware loads the model from the route and authorizes it,
// so the handler never runs for a denied request.
func TestCanMiddleware(t *testing.T) {
	kernel, _, _ := setupAuthApp(t)

	gate := auth.NewGate()
	require.NoError(t, auth.RegisterPolicy[User](gate, &UserPolicy{}))

	actingAs := int64(1)
	kernel.Use(func(ctx *genhttp.Context, next func() error) error {
		ctx.SetUser(&User{Model: database.Model{ID: actingAs}})
		return next()
	})

	handlerRan := false
	kernel.GET("/users/:user/edit", func(ctx *genhttp.Context) error {
		handlerRan = true
		return ctx.String("edit")
	}).Middleware(auth.Can[User](gate, "update", "user"))

	resp, err := kernel.Fiber().Test(httptest.NewRequest("GET", "/users/1/edit", nil), -1)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.True(t, handlerRan)

	// Acting as someone else, the same route is forbidden.
	actingAs = 2
	handlerRan = false
	resp, err = kernel.Fiber().Test(httptest.NewRequest("GET", "/users/1/edit", nil), -1)
	require.NoError(t, err)
	assert.Equal(t, 403, resp.StatusCode)
	assert.False(t, handlerRan, "the handler must not run for a denied request")
}

// A route model that does not exist is a 404, decided before the policy
// is asked about a model that is not there.
func TestCanMiddlewareMissingModelIs404(t *testing.T) {
	kernel, _, _ := setupAuthApp(t)

	gate := auth.NewGate()
	require.NoError(t, auth.RegisterPolicy[User](gate, &UserPolicy{}))

	kernel.Use(func(ctx *genhttp.Context, next func() error) error {
		ctx.SetUser(&User{Model: database.Model{ID: 1}})
		return next()
	})
	kernel.GET("/users/:user", func(ctx *genhttp.Context) error {
		return ctx.String("show")
	}).Middleware(auth.Can[User](gate, "update", "user"))

	resp, err := kernel.Fiber().Test(httptest.NewRequest("GET", "/users/999", nil), -1)
	require.NoError(t, err)
	assert.Equal(t, 404, resp.StatusCode)
}

func TestCanAnyMiddleware(t *testing.T) {
	kernel, _, _ := setupAuthApp(t)

	gate := auth.NewGate()
	require.NoError(t, auth.RegisterPolicy[User](gate, &UserPolicy{}))

	authenticated := true
	kernel.Use(func(ctx *genhttp.Context, next func() error) error {
		if authenticated {
			ctx.SetUser(&User{Model: database.Model{ID: 1}})
		}
		return next()
	})
	kernel.POST("/users", func(ctx *genhttp.Context) error {
		return ctx.String("created")
	}).Middleware(auth.CanAny[User](gate, "create"))

	resp, err := kernel.Fiber().Test(httptest.NewRequest("POST", "/users", nil), -1)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	authenticated = false
	resp, err = kernel.Fiber().Test(httptest.NewRequest("POST", "/users", nil), -1)
	require.NoError(t, err)
	assert.Equal(t, 403, resp.StatusCode)
}
