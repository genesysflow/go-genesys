package http

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

// TestingT is the subset of *testing.T the assertion helpers need.
type TestingT interface {
	Helper()
	Errorf(format string, args ...any)
}

// TestCase drives HTTP requests through a kernel with Laravel-style
// response assertions:
//
//	tc := http.NewTestCase(t, kernel)
//	tc.Get("/users").AssertOK().AssertJsonPath("data.0.name", "Ada")
type TestCase struct {
	t       TestingT
	kernel  *Kernel
	headers map[string]string

	// user is injected into every request by ActingAs.
	user     any
	injected bool

	// cookies is the jar carried between requests, so a session survives
	// a redirect the way a browser's would.
	cookies map[string]string
}

// NewTestCase creates a test case bound to a kernel.
func NewTestCase(t TestingT, kernel *Kernel) *TestCase {
	return &TestCase{t: t, kernel: kernel, headers: make(map[string]string)}
}

// WithHeader sends a default header with every request.
func (tc *TestCase) WithHeader(key, value string) *TestCase {
	tc.headers[key] = value
	return tc
}

// WithToken sends a bearer token with every request.
func (tc *TestCase) WithToken(token string) *TestCase {
	return tc.WithHeader("Authorization", "Bearer "+token)
}

// Do executes a TestRequest against the kernel.
func (tc *TestCase) Do(request *TestRequest) *TestResponse {
	for key, value := range tc.headers {
		if _, set := request.headers[key]; !set {
			request.headers[key] = value
		}
	}
	for name, value := range tc.cookies {
		if !request.hasCookie(name) {
			request.WithCookie(name, value)
		}
	}

	resp, err := tc.kernel.Fiber().Test(request.toHTTPRequest(), -1)
	if err != nil {
		tc.t.Helper()
		tc.t.Errorf("http test: request %s %s failed: %v", request.method, request.path, err)
		return &TestResponse{t: tc.t}
	}

	tc.rememberCookies(resp)

	response := newTestResponse(resp)
	response.t = tc.t
	return response
}

// Get performs a GET request.
func (tc *TestCase) Get(path string) *TestResponse {
	return tc.Do(Get(path))
}

// Post performs a POST request with a JSON body.
func (tc *TestCase) Post(path string, body ...any) *TestResponse {
	request := Post(path)
	if len(body) > 0 {
		request.WithJSON(body[0])
	}
	return tc.Do(request)
}

// Put performs a PUT request with a JSON body.
func (tc *TestCase) Put(path string, body ...any) *TestResponse {
	request := Put(path)
	if len(body) > 0 {
		request.WithJSON(body[0])
	}
	return tc.Do(request)
}

// Patch performs a PATCH request with a JSON body.
func (tc *TestCase) Patch(path string, body ...any) *TestResponse {
	request := Patch(path)
	if len(body) > 0 {
		request.WithJSON(body[0])
	}
	return tc.Do(request)
}

// Delete performs a DELETE request.
func (tc *TestCase) Delete(path string) *TestResponse {
	return tc.Do(Delete(path))
}

// PostForm performs a POST request with a form body.
func (tc *TestCase) PostForm(path string, data map[string]string) *TestResponse {
	return tc.Do(Post(path).WithForm(data))
}

// --- assertions ---

func (r *TestResponse) fail(format string, args ...any) *TestResponse {
	if r.t != nil {
		r.t.Helper()
		r.t.Errorf(format, args...)
	}
	return r
}

// AssertStatus asserts the response status code.
func (r *TestResponse) AssertStatus(expected int) *TestResponse {
	if r.resp == nil {
		return r.fail("http test: no response (the request itself failed)")
	}
	if r.resp.StatusCode != expected {
		return r.fail("expected status %d, got %d (body: %.200s)", expected, r.resp.StatusCode, r.body)
	}
	return r
}

// AssertOK asserts a 200 response.
func (r *TestResponse) AssertOK() *TestResponse { return r.AssertStatus(200) }

// AssertCreated asserts a 201 response.
func (r *TestResponse) AssertCreated() *TestResponse { return r.AssertStatus(201) }

// AssertNoContent asserts a 204 response.
func (r *TestResponse) AssertNoContent() *TestResponse { return r.AssertStatus(204) }

// AssertBadRequest asserts a 400 response.
func (r *TestResponse) AssertBadRequest() *TestResponse { return r.AssertStatus(400) }

// AssertUnauthorized asserts a 401 response.
func (r *TestResponse) AssertUnauthorized() *TestResponse { return r.AssertStatus(401) }

// AssertForbidden asserts a 403 response.
func (r *TestResponse) AssertForbidden() *TestResponse { return r.AssertStatus(403) }

// AssertNotFound asserts a 404 response.
func (r *TestResponse) AssertNotFound() *TestResponse { return r.AssertStatus(404) }

// AssertUnprocessable asserts a 422 response.
func (r *TestResponse) AssertUnprocessable() *TestResponse { return r.AssertStatus(422) }

// AssertRedirect asserts a redirect, optionally to a specific location.
func (r *TestResponse) AssertRedirect(location ...string) *TestResponse {
	if r.resp == nil {
		return r.fail("http test: no response")
	}
	if r.resp.StatusCode < 300 || r.resp.StatusCode > 399 {
		return r.fail("expected a redirect, got status %d", r.resp.StatusCode)
	}
	if len(location) > 0 {
		if got := r.resp.Header.Get("Location"); got != location[0] {
			return r.fail("expected redirect to %q, got %q", location[0], got)
		}
	}
	return r
}

// AssertHeader asserts a response header value.
func (r *TestResponse) AssertHeader(key, expected string) *TestResponse {
	if got := r.Header(key); got != expected {
		return r.fail("expected header %s=%q, got %q", key, expected, got)
	}
	return r
}

// AssertSee asserts the body contains the given text.
func (r *TestResponse) AssertSee(text string) *TestResponse {
	if !strings.Contains(string(r.body), text) {
		return r.fail("expected body to contain %q (body: %.200s)", text, r.body)
	}
	return r
}

// AssertDontSee asserts the body does not contain the given text.
func (r *TestResponse) AssertDontSee(text string) *TestResponse {
	if strings.Contains(string(r.body), text) {
		return r.fail("expected body not to contain %q", text)
	}
	return r
}

// AssertJson asserts that every key/value pair appears at the top level
// of the JSON response (extra keys are allowed).
func (r *TestResponse) AssertJson(subset map[string]any) *TestResponse {
	var decoded map[string]any
	if err := json.Unmarshal(r.body, &decoded); err != nil {
		return r.fail("response is not JSON: %v (body: %.200s)", err, r.body)
	}
	for key, expected := range subset {
		got, exists := decoded[key]
		if !exists {
			r.fail("expected JSON key %q (body: %.200s)", key, r.body)
			continue
		}
		if !jsonEqual(expected, got) {
			r.fail("expected JSON %q to be %v, got %v", key, expected, got)
		}
	}
	return r
}

// AssertJsonPath asserts a value at a dot-notation path, with numeric
// segments indexing arrays: "data.0.name".
func (r *TestResponse) AssertJsonPath(path string, expected any) *TestResponse {
	got, err := r.jsonPath(path)
	if err != nil {
		return r.fail("%v (body: %.200s)", err, r.body)
	}
	if !jsonEqual(expected, got) {
		return r.fail("expected JSON path %q to be %v, got %v", path, expected, got)
	}
	return r
}

// AssertJsonCount asserts the length of the array at a path ("" for the
// response root).
func (r *TestResponse) AssertJsonCount(path string, expected int) *TestResponse {
	got, err := r.jsonPath(path)
	if err != nil {
		return r.fail("%v", err)
	}
	list, ok := got.([]any)
	if !ok {
		return r.fail("expected JSON path %q to be an array, got %T", path, got)
	}
	if len(list) != expected {
		return r.fail("expected %d items at %q, got %d", expected, path, len(list))
	}
	return r
}

// AssertValidationError asserts the Laravel-shaped 422 payload has an
// error for the given field.
func (r *TestResponse) AssertValidationError(field string) *TestResponse {
	r.AssertStatus(422)
	got, err := r.jsonPath("errors." + field)
	if err != nil || got == nil {
		return r.fail("expected a validation error for %q (body: %.200s)", field, r.body)
	}
	return r
}

// jsonPath resolves a dot-notation path in the JSON body.
func (r *TestResponse) jsonPath(path string) (any, error) {
	var current any
	if err := json.Unmarshal(r.body, &current); err != nil {
		return nil, fmt.Errorf("response is not JSON: %w", err)
	}
	if path == "" {
		return current, nil
	}
	for _, segment := range strings.Split(path, ".") {
		switch node := current.(type) {
		case map[string]any:
			value, ok := node[segment]
			if !ok {
				return nil, fmt.Errorf("JSON path %q: key %q not found", path, segment)
			}
			current = value
		case []any:
			index, err := strconv.Atoi(segment)
			if err != nil || index < 0 || index >= len(node) {
				return nil, fmt.Errorf("JSON path %q: bad array index %q", path, segment)
			}
			current = node[index]
		default:
			return nil, fmt.Errorf("JSON path %q: cannot descend into %T at %q", path, current, segment)
		}
	}
	return current, nil
}

// jsonEqual compares an expected Go value with a decoded JSON value by
// normalizing both through JSON encoding.
func jsonEqual(expected, got any) bool {
	expectedJSON, err1 := json.Marshal(expected)
	gotJSON, err2 := json.Marshal(got)
	if err1 != nil || err2 != nil {
		return reflect.DeepEqual(expected, got)
	}
	var e, g any
	json.Unmarshal(expectedJSON, &e)
	json.Unmarshal(gotJSON, &g)
	return reflect.DeepEqual(e, g)
}
