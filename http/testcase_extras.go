package http

import (
	nethttp "net/http"
	"strings"
)

// ActingAs authenticates every subsequent request as the given user,
// Laravel's actingAs: a guarded route can be exercised without driving
// the real login flow.
//
// The user is injected by router middleware, which the test case
// registers once. Because router middleware is read at request time, it
// applies to routes registered before the test case was created too.
func (tc *TestCase) ActingAs(user any) *TestCase {
	tc.installUserInjector()
	tc.user = user
	return tc
}

// ActingAsGuest drops the authenticated user, so one test case can cover
// both sides of an authorization boundary.
func (tc *TestCase) ActingAsGuest() *TestCase {
	tc.user = nil
	return tc
}

// installUserInjector registers the middleware that sets the acting
// user, once per test case.
func (tc *TestCase) installUserInjector() {
	if tc.injected {
		return
	}
	tc.injected = true

	tc.kernel.Use(func(ctx *Context, next func() error) error {
		if tc.user != nil {
			ctx.SetUser(tc.user)
		}
		return next()
	})
}

// rememberCookies stores the cookies a response set, so the next request
// carries them the way a browser would - which is what makes a session
// survive a redirect in a test.
func (tc *TestCase) rememberCookies(response *nethttp.Response) {
	if response == nil {
		return
	}
	if tc.cookies == nil {
		tc.cookies = make(map[string]string)
	}
	for _, cookie := range response.Cookies() {
		if cookie.MaxAge < 0 || cookie.Value == "" {
			delete(tc.cookies, cookie.Name)
			continue
		}
		tc.cookies[cookie.Name] = cookie.Value
	}
}

// FlushCookies empties the cookie jar, isolating what follows from what
// came before.
func (tc *TestCase) FlushCookies() *TestCase {
	tc.cookies = nil
	return tc
}

// Cookie returns a cookie the jar holds.
func (tc *TestCase) Cookie(name string) string {
	return tc.cookies[name]
}

// AssertJsonMissing fails when the path is present in the JSON body,
// which is how a response is checked for a field that must not leak.
func (r *TestResponse) AssertJsonMissing(path string) *TestResponse {
	if r.t == nil {
		return r
	}
	r.t.Helper()

	if _, err := r.jsonPath(path); err == nil {
		return r.fail("expected JSON path %q to be absent, it is present", path)
	}
	return r
}

// AssertHeaderMissing fails when the header is present.
func (r *TestResponse) AssertHeaderMissing(key string) *TestResponse {
	if r.t == nil {
		return r
	}
	r.t.Helper()

	if value := r.Header(key); value != "" {
		return r.fail("expected no %s header, got %q", key, value)
	}
	return r
}

// AssertCookie fails unless the response set a cookie with the value.
func (r *TestResponse) AssertCookie(name, expected string) *TestResponse {
	if r.t == nil {
		return r
	}
	r.t.Helper()

	for _, cookie := range r.resp.Cookies() {
		if cookie.Name != name {
			continue
		}
		if cookie.Value != expected {
			return r.fail("expected cookie %s to be %q, got %q", name, expected, cookie.Value)
		}
		return r
	}
	return r.fail("expected the response to set the cookie %s", name)
}

// AssertLocationContains fails unless the Location header contains the
// fragment, for redirects whose target carries a generated id.
func (r *TestResponse) AssertLocationContains(fragment string) *TestResponse {
	if r.t == nil {
		return r
	}
	r.t.Helper()

	location := r.Header("Location")
	if !strings.Contains(location, fragment) {
		return r.fail("expected the redirect target to contain %q, got %q", fragment, location)
	}
	return r
}
