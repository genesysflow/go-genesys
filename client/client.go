// Package client provides a fluent HTTP client mirroring Laravel's Http
// facade:
//
//	resp, err := client.New().
//	    WithToken(token).
//	    Retry(3, time.Second).
//	    Get("https://api.example.com/users")
package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	nethttp "net/http"
	"net/url"
	"strings"
	"time"
)

// Request is a pending HTTP request under construction.
type Request struct {
	client     *nethttp.Client
	baseURL    string
	headers    map[string]string
	query      url.Values
	basicUser  string
	basicPass  string
	hasBasic   bool
	asForm     bool
	retries    int
	retryDelay time.Duration
}

// New creates a request builder with a 30-second timeout. Custom
// headers set on the builder are not forwarded to a different host on
// redirect, so API keys cannot leak to a redirect target.
func New() *Request {
	r := &Request{
		headers: make(map[string]string),
		query:   url.Values{},
	}
	r.client = &nethttp.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *nethttp.Request, via []*nethttp.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("client: stopped after 10 redirects")
			}
			// net/http only strips Authorization/Cookie cross-host; our
			// builder headers (e.g. X-Api-Key) must not travel either.
			if req.URL.Host != via[0].URL.Host {
				for key := range r.headers {
					req.Header.Del(key)
				}
			}
			return nil
		},
	}
	return r
}

// BaseURL prefixes all request URLs.
func (r *Request) BaseURL(base string) *Request {
	r.baseURL = strings.TrimRight(base, "/")
	return r
}

// Timeout sets the request timeout.
func (r *Request) Timeout(d time.Duration) *Request {
	r.client.Timeout = d
	return r
}

// WithHeader adds a header.
func (r *Request) WithHeader(key, value string) *Request {
	r.headers[key] = value
	return r
}

// WithHeaders adds multiple headers.
func (r *Request) WithHeaders(headers map[string]string) *Request {
	for k, v := range headers {
		r.headers[k] = v
	}
	return r
}

// WithToken sets a bearer token.
func (r *Request) WithToken(token string) *Request {
	r.headers["Authorization"] = "Bearer " + token
	return r
}

// WithBasicAuth sets basic authentication credentials.
func (r *Request) WithBasicAuth(username, password string) *Request {
	r.basicUser, r.basicPass, r.hasBasic = username, password, true
	return r
}

// WithQuery adds a query parameter.
func (r *Request) WithQuery(key, value string) *Request {
	r.query.Add(key, value)
	return r
}

// AsForm sends the body as application/x-www-form-urlencoded instead of JSON.
func (r *Request) AsForm() *Request {
	r.asForm = true
	return r
}

// Accept sets the Accept header.
func (r *Request) Accept(contentType string) *Request {
	r.headers["Accept"] = contentType
	return r
}

// Retry retries failed requests (network errors and 5xx responses) up to
// times with the given delay between attempts. Like Laravel's
// Http::retry it applies to every verb: a POST/PATCH whose first
// attempt reached the server but failed mid-response will be re-sent,
// so give non-idempotent endpoints an idempotency key before retrying
// them.
func (r *Request) Retry(times int, delay time.Duration) *Request {
	r.retries = times
	r.retryDelay = delay
	return r
}

// Get sends a GET request.
func (r *Request) Get(target string) (*Response, error) {
	return r.send("GET", target, nil)
}

// Post sends a POST request with the given body.
func (r *Request) Post(target string, body ...any) (*Response, error) {
	return r.send("POST", target, first(body))
}

// Put sends a PUT request with the given body.
func (r *Request) Put(target string, body ...any) (*Response, error) {
	return r.send("PUT", target, first(body))
}

// Patch sends a PATCH request with the given body.
func (r *Request) Patch(target string, body ...any) (*Response, error) {
	return r.send("PATCH", target, first(body))
}

// Delete sends a DELETE request.
func (r *Request) Delete(target string, body ...any) (*Response, error) {
	return r.send("DELETE", target, first(body))
}

func first(body []any) any {
	if len(body) > 0 {
		return body[0]
	}
	return nil
}

func (r *Request) send(method, target string, body any) (*Response, error) {
	if r.baseURL != "" && !strings.HasPrefix(target, "http") {
		target = r.baseURL + "/" + strings.TrimLeft(target, "/")
	}
	if len(r.query) > 0 {
		separator := "?"
		if strings.Contains(target, "?") {
			separator = "&"
		}
		target += separator + r.query.Encode()
	}

	attempts := r.retries + 1
	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 && r.retryDelay > 0 {
			time.Sleep(r.retryDelay)
		}

		payload, contentType, err := r.encodeBody(body)
		if err != nil {
			return nil, err
		}

		req, err := nethttp.NewRequest(method, target, payload)
		if err != nil {
			return nil, err
		}
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		for k, v := range r.headers {
			req.Header.Set(k, v)
		}
		if r.hasBasic {
			req.SetBasicAuth(r.basicUser, r.basicPass)
		}

		resp, err := r.client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		raw, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}

		response := &Response{raw: resp, body: raw}
		if response.ServerError() && attempt < attempts-1 {
			lastErr = fmt.Errorf("client: server returned %d", resp.StatusCode)
			continue
		}
		return response, nil
	}
	return nil, fmt.Errorf("client: request failed after %d attempt(s): %w", attempts, lastErr)
}

// encodeBody converts the body to a reader plus content type.
func (r *Request) encodeBody(body any) (io.Reader, string, error) {
	if body == nil {
		return nil, "", nil
	}
	switch v := body.(type) {
	case string:
		return strings.NewReader(v), "", nil
	case []byte:
		return bytes.NewReader(v), "", nil
	case url.Values:
		return strings.NewReader(v.Encode()), "application/x-www-form-urlencoded", nil
	}

	if r.asForm {
		values, ok := body.(map[string]string)
		if !ok {
			return nil, "", fmt.Errorf("client: AsForm requires map[string]string or url.Values body, got %T", body)
		}
		form := url.Values{}
		for k, v := range values {
			form.Set(k, v)
		}
		return strings.NewReader(form.Encode()), "application/x-www-form-urlencoded", nil
	}

	raw, err := json.Marshal(body)
	if err != nil {
		return nil, "", fmt.Errorf("client: cannot encode body as JSON: %w", err)
	}
	return bytes.NewReader(raw), "application/json", nil
}

// Response wraps an HTTP response with convenience accessors.
type Response struct {
	raw  *nethttp.Response
	body []byte
}

// Status returns the HTTP status code.
func (r *Response) Status() int {
	return r.raw.StatusCode
}

// Body returns the raw response body.
func (r *Response) Body() []byte {
	return r.body
}

// String returns the response body as a string.
func (r *Response) String() string {
	return string(r.body)
}

// JSON unmarshals the response body into v.
func (r *Response) JSON(v any) error {
	return json.Unmarshal(r.body, v)
}

// Header returns a response header value.
func (r *Response) Header(key string) string {
	return r.raw.Header.Get(key)
}

// Successful reports whether the status is 2xx.
func (r *Response) Successful() bool {
	return r.Status() >= 200 && r.Status() < 300
}

// Failed reports whether the status is 4xx or 5xx.
func (r *Response) Failed() bool {
	return r.Status() >= 400
}

// ClientError reports whether the status is 4xx.
func (r *Response) ClientError() bool {
	return r.Status() >= 400 && r.Status() < 500
}

// ServerError reports whether the status is 5xx.
func (r *Response) ServerError() bool {
	return r.Status() >= 500
}

// Get sends a GET request with default settings.
func Get(target string) (*Response, error) {
	return New().Get(target)
}

// Post sends a POST request with default settings.
func Post(target string, body ...any) (*Response, error) {
	return New().Post(target, body...)
}
