package http

import "github.com/genesysflow/go-genesys/errors"

// contextGateKey is the store key holding the request's authorizer.
const contextGateKey = "genesys.auth.gate"

// Authorizer answers authorization questions for the current request's
// user. The auth package binds one per request; http keeps the surface
// to this interface so it does not depend on the auth package (which
// depends on it).
type Authorizer interface {
	// Allows reports whether the current user may perform the ability.
	// args carry the subject, e.g. the model being acted on.
	Allows(ability string, args ...any) bool
}

// denyAll is the authorizer used when none was bound: with no gate to
// ask, the answer to "may they?" is no.
type denyAll struct{}

func (denyAll) Allows(ability string, args ...any) bool { return false }

// SetGate binds the authorizer for this request. auth.GateMiddleware
// calls it; handlers rarely need to.
func (c *Context) SetGate(authorizer Authorizer) {
	c.Set(contextGateKey, authorizer)
}

// Gate returns the request's authorizer, which is never nil: with no
// gate bound it denies everything, so a missing middleware fails closed
// rather than authorizing by accident.
func (c *Context) Gate() Authorizer {
	if authorizer, ok := c.Get(contextGateKey).(Authorizer); ok && authorizer != nil {
		return authorizer
	}
	return denyAll{}
}

// Can reports whether the current user may perform the ability:
//
//	if ctx.Can("update", post) { ... }
func (c *Context) Can(ability string, args ...any) bool {
	return c.Gate().Allows(ability, args...)
}

// Cannot is the negation of Can.
func (c *Context) Cannot(ability string, args ...any) bool {
	return !c.Can(ability, args...)
}

// Authorize returns a 403 error unless the current user may perform the
// ability, so a handler can guard itself in one line:
//
//	if err := ctx.Authorize("update", post); err != nil {
//	    return err
//	}
func (c *Context) Authorize(ability string, args ...any) error {
	if c.Can(ability, args...) {
		return nil
	}
	return errors.Forbidden("This action is unauthorized.")
}
