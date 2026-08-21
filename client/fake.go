package client

import (
	"fmt"
	"io"
	nethttp "net/http"
	"strings"
	"sync"
)

// Fake stubs HTTP responses and records requests - Laravel's
// Http::fake(). Attach it to a request builder and every send is
// answered from the stubs instead of the network:
//
//	fake := client.NewFake().
//	    Respond("https://api.example.com/users*", 200, `[{"id":1}]`).
//	    Respond("*", 404, "")
//
//	resp, _ := client.New().WithFake(fake).Get("https://api.example.com/users")
//	fake.AssertSent(t, "GET", "https://api.example.com/users*")
type Fake struct {
	mu       sync.Mutex
	stubs    []fakeStub
	recorded []RecordedRequest
}

type fakeStub struct {
	pattern string
	status  int
	body    string
	headers map[string]string
}

// RecordedRequest is one request the fake intercepted.
type RecordedRequest struct {
	Method string
	URL    string
	Header nethttp.Header
	Body   string
}

// NewFake creates an empty fake; unmatched requests get 200 "".
func NewFake() *Fake {
	return &Fake{}
}

// Respond stubs responses for URLs matching pattern. Patterns match
// exactly, or by prefix with a trailing "*"; "*" alone matches
// everything. Stubs are consulted in registration order.
func (f *Fake) Respond(pattern string, status int, body string, headers ...map[string]string) *Fake {
	stub := fakeStub{pattern: pattern, status: status, body: body}
	if len(headers) > 0 {
		stub.headers = headers[0]
	}
	f.mu.Lock()
	f.stubs = append(f.stubs, stub)
	f.mu.Unlock()
	return f
}

func matchesPattern(pattern, url string) bool {
	if pattern == "*" {
		return true
	}
	if prefix, ok := strings.CutSuffix(pattern, "*"); ok {
		return strings.HasPrefix(url, prefix)
	}
	return pattern == url
}

// RoundTrip implements http.RoundTripper: record, match, answer.
func (f *Fake) RoundTrip(req *nethttp.Request) (*nethttp.Response, error) {
	body := ""
	if req.Body != nil {
		raw, _ := io.ReadAll(req.Body)
		req.Body.Close()
		body = string(raw)
	}

	f.mu.Lock()
	f.recorded = append(f.recorded, RecordedRequest{
		Method: req.Method,
		URL:    req.URL.String(),
		Header: req.Header.Clone(),
		Body:   body,
	})
	stubs := append([]fakeStub(nil), f.stubs...)
	f.mu.Unlock()

	status, respBody := 200, ""
	var headers map[string]string
	for _, stub := range stubs {
		if matchesPattern(stub.pattern, req.URL.String()) {
			status, respBody, headers = stub.status, stub.body, stub.headers
			break
		}
	}

	resp := &nethttp.Response{
		StatusCode: status,
		Status:     fmt.Sprintf("%d %s", status, nethttp.StatusText(status)),
		Body:       io.NopCloser(strings.NewReader(respBody)),
		Header:     nethttp.Header{},
		Request:    req,
	}
	for k, v := range headers {
		resp.Header.Set(k, v)
	}
	return resp, nil
}

// Requests returns everything sent through the fake, in order.
func (f *Fake) Requests() []RecordedRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]RecordedRequest(nil), f.recorded...)
}

// testingT is the minimal testing surface the assertions need.
type testingT interface {
	Helper()
	Errorf(format string, args ...any)
}

// AssertSent fails the test unless a request matching method and URL
// pattern was sent.
func (f *Fake) AssertSent(t testingT, method, pattern string) {
	t.Helper()
	for _, r := range f.Requests() {
		if strings.EqualFold(r.Method, method) && matchesPattern(pattern, r.URL) {
			return
		}
	}
	t.Errorf("client: expected a %s request matching %q; got %d request(s)", method, pattern, len(f.Requests()))
}

// AssertNotSent fails the test when a matching request was sent.
func (f *Fake) AssertNotSent(t testingT, method, pattern string) {
	t.Helper()
	for _, r := range f.Requests() {
		if strings.EqualFold(r.Method, method) && matchesPattern(pattern, r.URL) {
			t.Errorf("client: unexpected %s request to %s", r.Method, r.URL)
			return
		}
	}
}

// AssertNothingSent fails the test when any request was sent.
func (f *Fake) AssertNothingSent(t testingT) {
	t.Helper()
	if n := len(f.Requests()); n > 0 {
		t.Errorf("client: expected no requests, got %d", n)
	}
}

// WithFake routes this builder's requests through the fake instead of
// the network.
func (r *Request) WithFake(fake *Fake) *Request {
	r.client.Transport = fake
	return r
}
