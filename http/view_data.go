package http

import "github.com/genesysflow/go-genesys/session"

// csrfContextKey mirrors middleware.CSRFContextKey. It is duplicated
// rather than imported because http/middleware imports http; the two must
// stay in step, and the middleware package's test asserts the shared key.
const csrfContextKey = "csrf_token"

// OldInput exposes input flashed by the previous request to templates:
//
//	<input name="email" value="{{.old.Get "email"}}">
type OldInput struct {
	sess *session.Session
}

// Get returns the flashed value for key, or the fallback when absent.
func (o OldInput) Get(key string, defaultValue ...string) string {
	if o.sess == nil {
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return ""
	}
	return o.sess.Old(key, defaultValue...)
}

// Has reports whether any input was flashed for key.
//
// Asked of the session by key rather than by comparing against a
// sentinel value: a flashed value that happened to equal the sentinel
// would otherwise report absent.
func (o OldInput) Has(key string) bool {
	if o.sess == nil {
		return false
	}
	return o.sess.HasOld(key)
}

// Any reports whether the previous request flashed any input at all.
func (o OldInput) Any() bool {
	return o.sess != nil && o.sess.HasOldInput()
}

// shareViewData layers the per-request values every template can rely on
// under the handler's own data, Laravel's shared `$errors` plus the
// helpers a form needs. Handler-supplied keys win, and the handler's map
// is never mutated - a map reused across requests would otherwise leak
// one request's user into the next.
func (c *Context) shareViewData(data map[string]any) map[string]any {
	sess := c.Session()

	merged := map[string]any{
		"errors":  c.Errors(),
		"old":     OldInput{sess: sess},
		"session": sess,
		"user":    c.User(),
		"gate":    c.Gate(),
	}
	if token, ok := c.Get(csrfContextKey).(string); ok {
		merged[csrfContextKey] = token
	}

	for key, value := range data {
		merged[key] = value
	}
	return merged
}

// ViewData returns the per-request values shared with every template,
// merged under the given data. Handlers rarely need it; it exists so
// middleware and tests can see what a view will receive.
func (c *Context) ViewData(data ...map[string]any) map[string]any {
	if len(data) > 0 {
		return c.shareViewData(data[0])
	}
	return c.shareViewData(nil)
}
