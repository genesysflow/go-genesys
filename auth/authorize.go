package auth

import (
	"github.com/genesysflow/go-genesys/errors"
	"github.com/genesysflow/go-genesys/http"
)

// requestGate answers authorization questions for one request's user.
type requestGate struct {
	gate *Gate
	ctx  *http.Context
}

// Allows resolves the user from the request each time, so a login that
// happens mid-request is reflected.
func (r requestGate) Allows(ability string, args ...any) bool {
	return r.gate.Allows(UserFrom(r.ctx), ability, args...)
}

// GateMiddleware binds the gate to every request, so handlers and views
// can ask ctx.Can("update", post) without threading the gate and the
// user through themselves.
//
// It does not authenticate: put it after the auth middleware (or after
// whatever sets the user) so the user is there to authorize.
func GateMiddleware(gate *Gate) http.MiddlewareFunc {
	return func(ctx *http.Context, next func() error) error {
		ctx.SetGate(requestGate{gate: gate, ctx: ctx})
		return next()
	}
}

// Can guards a route with an ability on a route-bound model, Laravel's
// `can:update,post` middleware:
//
//	router.PUT("/posts/:post", UpdatePost).
//	    Middleware(auth.Can[models.Post](gate, "update", "post"))
//
// The model is loaded from the route parameter; a missing one is a 404,
// and a denied ability is a 403.
func Can[T any](gate *Gate, ability, param string) http.MiddlewareFunc {
	return func(ctx *http.Context, next func() error) error {
		model, err := http.BindModel[T](ctx, param)
		if err != nil {
			return err
		}

		if !gate.Allows(UserFrom(ctx), ability, model) {
			return errors.Forbidden("This action is unauthorized.")
		}
		return next()
	}
}

// CanBy is Can for a route addressed by a column other than the primary
// key - a slug, a uuid, an email:
//
//	router.PUT("/posts/:post", UpdatePost).
//	    Middleware(auth.CanBy[models.Post](gate, "update", "post", "slug"))
//
// The column is a schema name, not user input; the value from the route
// is bound, never interpolated.
func CanBy[T any](gate *Gate, ability, param, column string) http.MiddlewareFunc {
	return func(ctx *http.Context, next func() error) error {
		model, err := http.BindModelBy[T](ctx, param, column)
		if err != nil {
			return err
		}

		if !gate.Allows(UserFrom(ctx), ability, model) {
			return errors.Forbidden("This action is unauthorized.")
		}
		return next()
	}
}

// CanAny guards a route with an instance-less ability, such as "create"
// or "view-any":
//
//	router.POST("/posts", StorePost).
//	    Middleware(auth.CanAny[models.Post](gate, "create"))
func CanAny[T any](gate *Gate, ability string) http.MiddlewareFunc {
	return func(ctx *http.Context, next func() error) error {
		if !AllowsFor[T](gate, UserFrom(ctx), ability) {
			return errors.Forbidden("This action is unauthorized.")
		}
		return next()
	}
}
