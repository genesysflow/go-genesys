package http

import (
	"strings"

	"github.com/genesysflow/go-genesys/session"
)

// contextUserKey is the store key holding the authenticated user. Keeping
// it unexported means only SetUser can populate it.
const contextUserKey = "genesys.auth.user"

// contextRouteKey is the store key holding the matched *Route.
const contextRouteKey = "genesys.route"

// SetUser stashes the authenticated user on the request. Guards and the
// auth middleware call this once per request so handlers do not have to
// re-resolve the user from the session or token on every access.
func (c *Context) SetUser(user any) {
	c.Set(contextUserKey, user)
}

// User returns the authenticated user stashed by the auth middleware, or
// nil when the request is unauthenticated. The concrete type is the user
// model the guard resolved; use UserAs for a typed accessor.
func (c *Context) User() any {
	return c.Get(contextUserKey)
}

// HasUser reports whether an authenticated user is attached to the request.
func (c *Context) HasUser() bool {
	return c.User() != nil
}

// UserAs returns the authenticated user as *T, Laravel's typed
// `$request->user()`:
//
//	user, ok := http.UserAs[models.User](ctx)
//
// The second return is false when the request is unauthenticated or the
// user is not a *T. Applications usually wrap it once, so no handler
// names the type parameter:
//
//	func CurrentUser(ctx *http.Context) *models.User {
//	    user, _ := http.UserAs[models.User](ctx)
//	    return user
//	}
func UserAs[T any](c *Context) (*T, bool) {
	user, ok := c.User().(*T)
	return user, ok
}

// Session returns the session for this request, or nil when no session
// middleware ran. Handlers can flash data, read old input, and regenerate
// the session without reaching through to Fiber.
func (c *Context) Session() *session.Session {
	if c.fiberCtx == nil {
		return nil
	}
	return session.GetFromContext(c.fiberCtx)
}

// Old returns input flashed on the previous request, Laravel's `old()`.
// It returns the fallback (or an empty string) when there is no session or
// no value under key.
func (c *Context) Old(key string, defaultValue ...string) string {
	sess := c.Session()
	if sess == nil {
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return ""
	}
	return sess.Old(key, defaultValue...)
}

// setRoute records the route that matched this request.
func (c *Context) setRoute(route *Route) {
	c.Set(contextRouteKey, route)
}

// Route returns the route that matched this request, or nil for handlers
// registered outside the router (fallbacks, raw Fiber middleware).
func (c *Context) Route() *Route {
	route, _ := c.Get(contextRouteKey).(*Route)
	return route
}

// RouteName returns the matched route's name, or an empty string when the
// route is unnamed or unknown.
func (c *Context) RouteName() string {
	if route := c.Route(); route != nil {
		return route.GetName()
	}
	return ""
}

// RouteIs reports whether the current route's name matches any of the
// given patterns, Laravel's `$request->routeIs('users.*')`. A trailing `*`
// matches any suffix. Unnamed routes match nothing.
func (c *Context) RouteIs(patterns ...string) bool {
	name := c.RouteName()
	if name == "" {
		return false
	}
	for _, pattern := range patterns {
		if matchRouteName(pattern, name) {
			return true
		}
	}
	return false
}

// matchRouteName matches a route name against a pattern supporting `*`
// wildcards anywhere in the pattern.
func matchRouteName(pattern, name string) bool {
	if pattern == name {
		return true
	}
	if !strings.Contains(pattern, "*") {
		return false
	}

	segments := strings.Split(pattern, "*")
	rest := name

	// The pattern's leading literal must prefix the name.
	if segments[0] != "" {
		if !strings.HasPrefix(rest, segments[0]) {
			return false
		}
		rest = rest[len(segments[0]):]
	}

	last := len(segments) - 1
	for i := 1; i < last; i++ {
		if segments[i] == "" {
			continue
		}
		index := strings.Index(rest, segments[i])
		if index < 0 {
			return false
		}
		rest = rest[index+len(segments[i]):]
	}

	// The pattern's trailing literal must suffix what is left.
	if last > 0 && segments[last] != "" {
		return strings.HasSuffix(rest, segments[last])
	}
	return true
}
