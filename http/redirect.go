package http

import (
	"encoding/gob"
	stderrors "errors"
	"fmt"

	"github.com/genesysflow/go-genesys/container"
	"github.com/genesysflow/go-genesys/errors"
	"github.com/genesysflow/go-genesys/support"
)

func init() {
	// Errors are flashed as a plain map so serializing session storages
	// (file, database, redis) can encode them.
	gob.Register(map[string][]string{})
}

// sessionErrorsKey is the session key holding the flashed error bag,
// Laravel's `$errors`.
const sessionErrorsKey = "errors"

// dontFlash are input fields never written to session storage. Flashing a
// password would persist a plaintext credential to disk or Redis for the
// lifetime of the session, so old input drops them the way Laravel's
// exception handler does.
var dontFlash = map[string]bool{
	"password":              true,
	"password_confirmation": true,
	"current_password":      true,
	"new_password":          true,
}

// Redirector builds a redirect response, optionally flashing errors, old
// input, and one-off session values first. It is Laravel's
// `redirect()->back()->withErrors(...)->withInput()`:
//
//	return ctx.Back().WithErrors(err).WithInput().Send()
//
// Nothing happens until Send is called; because handlers must return an
// error, forgetting Send is a compile error rather than a silent no-op.
type Redirector struct {
	ctx      *Context
	target   string
	fallback string
	back     bool
	status   int
	err      error

	flashes  map[string]any
	errors   map[string][]string
	input    map[string]any
	hasInput bool
}

// RedirectTo starts a redirect to an absolute path or URL.
func (c *Context) RedirectTo(url string) *Redirector {
	return &Redirector{ctx: c, target: url, status: 302}
}

// Back starts a redirect to the previous page (the Referer when it points
// at this host), falling back to the given path or "/".
func (c *Context) Back(fallback ...string) *Redirector {
	r := &Redirector{ctx: c, back: true, status: 302, fallback: "/"}
	if len(fallback) > 0 && fallback[0] != "" {
		r.fallback = fallback[0]
	}
	return r
}

// intendedSessionKey holds the URL a guest was trying to reach.
const intendedSessionKey = "url.intended"

// SetIntendedURL remembers where the visitor was headed, so they can be
// sent on after signing in. The auth middleware records it before
// sending a guest to the login page.
//
// Nothing is validated here: the value is checked when it is used, so a
// stored URL cannot become trusted by sitting in the session.
func (c *Context) SetIntendedURL(target string) {
	session := c.Session()
	if session == nil || target == "" {
		return
	}
	_ = session.Set(intendedSessionKey, target)
}

// Intended redirects to where the visitor was headed before they were
// asked to sign in, falling back to the given path - Laravel's
// redirect()->intended().
//
// The stored destination came from the request, so it is honoured only
// when it stays on this host: this is the redirect an application would
// otherwise write as RedirectTo(ctx.Query("next")), which is an open
// redirect. It is spent once, so a later login lands on the fallback.
func (c *Context) Intended(fallback string) *Redirector {
	if fallback == "" {
		fallback = "/"
	}

	target := fallback
	if session := c.Session(); session != nil {
		if stored, ok := session.Get(intendedSessionKey).(string); ok && stored != "" {
			if sameHostRedirect(stored, c.fiberCtx.Hostname()) {
				target = stored
			}
		}
		session.Forget(intendedSessionKey)
	}

	return &Redirector{ctx: c, target: target, status: 302}
}

// RedirectToRoute starts a redirect to a named route. Send reports an
// error when no route carries that name.
func (c *Context) RedirectToRoute(name string, params ...map[string]any) *Redirector {
	r := &Redirector{ctx: c, status: 302}

	router := c.router()
	if router == nil {
		r.err = fmt.Errorf("redirect: no router available to resolve route %q", name)
		return r
	}

	target := router.URL(name, params...)
	if target == "" {
		r.err = fmt.Errorf("redirect: route %q is not defined", name)
		return r
	}

	r.target = target
	return r
}

// router resolves the router backing this request: the one that matched
// the route when available, otherwise the container's.
func (c *Context) router() *Router {
	if route := c.Route(); route != nil && route.router != nil {
		return route.router
	}
	if router := routerFromCtx(c.fiberCtx); router != nil {
		return router
	}
	if c.app == nil {
		return nil
	}
	router, err := container.Resolve[*Router](c.app, "router")
	if err != nil {
		return nil
	}
	return router
}

// Status overrides the redirect status code (302 by default).
func (r *Redirector) Status(code int) *Redirector {
	r.status = code
	return r
}

// With flashes a value for the next request, Laravel's
// `->with('status', 'Saved!')`.
func (r *Redirector) With(key string, value any) *Redirector {
	if r.flashes == nil {
		r.flashes = make(map[string]any)
	}
	r.flashes[key] = value
	return r
}

// WithErrors flashes validation errors for the next request, where they
// are read back as ctx.Errors() and shared with views as `errors`.
//
// It accepts the shapes handlers actually hold: map[string][]string,
// *errors.ValidationError, *support.MessageBag, or any plain error (whose
// message lands under the "message" key).
func (r *Redirector) WithErrors(errs any) *Redirector {
	if r.errors == nil {
		r.errors = make(map[string][]string)
	}

	for field, messages := range messagesFrom(errs) {
		r.errors[field] = append(r.errors[field], messages...)
	}
	return r
}

// messagesFrom normalises the supported error shapes to field messages.
func messagesFrom(errs any) map[string][]string {
	switch typed := errs.(type) {
	case nil:
		return nil
	case map[string][]string:
		return typed
	case map[string]string:
		out := make(map[string][]string, len(typed))
		for field, message := range typed {
			out[field] = []string{message}
		}
		return out
	case *support.MessageBag:
		if typed == nil {
			return nil
		}
		return typed.Messages()
	case *errors.ValidationError:
		if typed == nil {
			return nil
		}
		return typed.Errors
	case error:
		var validationErr *errors.ValidationError
		if stderrors.As(typed, &validationErr) {
			return validationErr.Errors
		}
		return map[string][]string{"message": {typed.Error()}}
	case string:
		return map[string][]string{"message": {typed}}
	default:
		return map[string][]string{"message": {fmt.Sprint(typed)}}
	}
}

// WithInput flashes input so the form can be repopulated on the next
// request via ctx.Old(). With no argument it flashes the current request's
// input, minus password fields.
func (r *Redirector) WithInput(input ...map[string]any) *Redirector {
	r.hasInput = true
	if len(input) > 0 {
		r.input = input[0]
		return r
	}

	all := r.ctx.All()
	filtered := make(map[string]any, len(all))
	for key, value := range all {
		if dontFlash[key] {
			continue
		}
		filtered[key] = value
	}
	r.input = filtered
	return r
}

// Send flashes anything queued and issues the redirect.
func (r *Redirector) Send() error {
	if r.err != nil {
		return r.err
	}

	if sess := r.ctx.Session(); sess != nil {
		if len(r.errors) > 0 {
			if err := sess.Flash(sessionErrorsKey, r.errors); err != nil {
				return err
			}
		}
		if r.hasInput {
			if err := sess.FlashInput(r.input); err != nil {
				return err
			}
		}
		for key, value := range r.flashes {
			if err := sess.Flash(key, value); err != nil {
				return err
			}
		}
	}

	target := r.target
	if r.back {
		// The Referer is attacker-controlled, so it is only honoured
		// when it points at this same host; otherwise every handler
		// redirecting back would be an open redirect.
		target = r.fallback
		if referer := r.ctx.fiberCtx.Get("Referer"); sameHostRedirect(referer, r.ctx.fiberCtx.Hostname()) {
			target = referer
		}
	}
	return r.ctx.response.Redirect(target, r.status)
}

// Errors returns the validation errors flashed by the previous request.
// The bag is never nil, so handlers and templates can call into it
// without a guard.
func (c *Context) Errors() *support.MessageBag {
	sess := c.Session()
	if sess == nil {
		return support.NewMessageBag(nil)
	}

	messages, _ := sess.Get(sessionErrorsKey).(map[string][]string)
	return support.NewMessageBag(messages)
}

// presentValidationFailure renders a validation failure the way the
// client can use it. Browsers get Laravel's redirect back to the form
// with the messages and their input flashed; API clients, and browsers
// with no session to flash into, fall through to the 422 JSON envelope.
//
// This runs inside the middleware chain rather than in the top-level
// error handler because session middleware wraps the chain from the
// outside: flashing after it has already saved would drop the errors.
func presentValidationFailure(ctx *Context, err error) (bool, error) {
	var validationErr *errors.ValidationError
	if !stderrors.As(err, &validationErr) {
		return false, nil
	}

	req := ctx.request
	if req.IsJSON() || req.IsAjax() || !req.Accepts("text/html") {
		return false, nil
	}

	if ctx.Session() == nil {
		return false, nil
	}

	return true, ctx.Back().WithErrors(validationErr).WithInput().Send()
}
