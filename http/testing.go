package http

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
)

// TestRequest represents a test HTTP request.
type TestRequest struct {
	method  string
	path    string
	headers map[string]string
	body    []byte
	cookies []*http.Cookie
}

// NewTestRequest creates a new test request.
func NewTestRequest(method, path string) *TestRequest {
	return &TestRequest{
		method:  method,
		path:    path,
		headers: make(map[string]string),
		cookies: make([]*http.Cookie, 0),
	}
}

// Get creates a GET test request.
func Get(path string) *TestRequest {
	return NewTestRequest("GET", path)
}

// Post creates a POST test request.
func Post(path string) *TestRequest {
	return NewTestRequest("POST", path)
}

// Put creates a PUT test request.
func Put(path string) *TestRequest {
	return NewTestRequest("PUT", path)
}

// Patch creates a PATCH test request.
func Patch(path string) *TestRequest {
	return NewTestRequest("PATCH", path)
}

// Delete creates a DELETE test request.
func Delete(path string) *TestRequest {
	return NewTestRequest("DELETE", path)
}

// WithHeader adds a header to the request.
func (r *TestRequest) WithHeader(key, value string) *TestRequest {
	r.headers[key] = value
	return r
}

// WithHeaders adds multiple headers to the request.
func (r *TestRequest) WithHeaders(headers map[string]string) *TestRequest {
	for k, v := range headers {
		r.headers[k] = v
	}
	return r
}

// WithBody sets the request body.
func (r *TestRequest) WithBody(body []byte) *TestRequest {
	r.body = body
	return r
}

// WithJSON sets the request body as JSON.
func (r *TestRequest) WithJSON(v any) *TestRequest {
	data, err := json.Marshal(v)
	if err == nil {
		r.body = data
		r.headers["Content-Type"] = "application/json"
	}
	return r
}

// WithForm sets the request body as form data.
func (r *TestRequest) WithForm(data map[string]string) *TestRequest {
	values := ""
	for k, v := range data {
		if values != "" {
			values += "&"
		}
		values += k + "=" + v
	}
	r.body = []byte(values)
	r.headers["Content-Type"] = "application/x-www-form-urlencoded"
	return r
}

// WithCookie adds a cookie to the request.
func (r *TestRequest) WithCookie(name, value string) *TestRequest {
	r.cookies = append(r.cookies, &http.Cookie{Name: name, Value: value})
	return r
}

// WithBearerToken adds a bearer token to the request.
func (r *TestRequest) WithBearerToken(token string) *TestRequest {
	r.headers["Authorization"] = "Bearer " + token
	return r
}

// WithBasicAuth adds basic auth to the request.
func (r *TestRequest) WithBasicAuth(username, password string) *TestRequest {
	// In a real implementation, this would base64 encode the credentials
	r.headers["Authorization"] = "Basic " + username + ":" + password
	return r
}

// toHTTPRequest converts to a standard http.Request.
func (r *TestRequest) toHTTPRequest() *http.Request {
	var bodyReader io.Reader
	if len(r.body) > 0 {
		bodyReader = bytes.NewReader(r.body)
	}

	req := httptest.NewRequest(r.method, r.path, bodyReader)

	for k, v := range r.headers {
		req.Header.Set(k, v)
	}

	for _, cookie := range r.cookies {
		req.AddCookie(cookie)
	}

	return req
}

// TestResponse represents a test HTTP response.
type TestResponse struct {
	resp *http.Response
	body []byte
}

// newTestResponse creates a new test response.
func newTestResponse(resp *http.Response) *TestResponse {
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return &TestResponse{
		resp: resp,
		body: body,
	}
}

// Status returns the response status code.
func (r *TestResponse) Status() int {
	return r.resp.StatusCode
}

// Body returns the response body.
func (r *TestResponse) Body() []byte {
	return r.body
}

// BodyString returns the response body as string.
func (r *TestResponse) BodyString() string {
	return string(r.body)
}

// Header returns a response header.
func (r *TestResponse) Header(key string) string {
	return r.resp.Header.Get(key)
}

// Headers returns all response headers.
func (r *TestResponse) Headers() http.Header {
	return r.resp.Header
}

// JSON parses the response body as JSON.
func (r *TestResponse) JSON(v any) error {
	return json.Unmarshal(r.body, v)
}

// Cookies returns all response cookies.
func (r *TestResponse) Cookies() []*http.Cookie {
	return r.resp.Cookies()
}

// Cookie returns a specific cookie.
func (r *TestResponse) Cookie(name string) *http.Cookie {
	for _, c := range r.resp.Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// IsOK returns true if status is 200.
func (r *TestResponse) IsOK() bool {
	return r.resp.StatusCode == http.StatusOK
}

// IsCreated returns true if status is 201.
func (r *TestResponse) IsCreated() bool {
	return r.resp.StatusCode == http.StatusCreated
}

// IsNoContent returns true if status is 204.
func (r *TestResponse) IsNoContent() bool {
	return r.resp.StatusCode == http.StatusNoContent
}

// IsBadRequest returns true if status is 400.
func (r *TestResponse) IsBadRequest() bool {
	return r.resp.StatusCode == http.StatusBadRequest
}

// IsUnauthorized returns true if status is 401.
func (r *TestResponse) IsUnauthorized() bool {
	return r.resp.StatusCode == http.StatusUnauthorized
}

// IsForbidden returns true if status is 403.
func (r *TestResponse) IsForbidden() bool {
	return r.resp.StatusCode == http.StatusForbidden
}

// IsNotFound returns true if status is 404.
func (r *TestResponse) IsNotFound() bool {
	return r.resp.StatusCode == http.StatusNotFound
}

// IsInternalServerError returns true if status is 500.
func (r *TestResponse) IsInternalServerError() bool {
	return r.resp.StatusCode == http.StatusInternalServerError
}

// IsRedirect returns true if status is a redirect (3xx).
func (r *TestResponse) IsRedirect() bool {
	return r.resp.StatusCode >= 300 && r.resp.StatusCode < 400
}

// IsSuccess returns true if status is successful (2xx).
func (r *TestResponse) IsSuccess() bool {
	return r.resp.StatusCode >= 200 && r.resp.StatusCode < 300
}

// IsClientError returns true if status is a client error (4xx).
func (r *TestResponse) IsClientError() bool {
	return r.resp.StatusCode >= 400 && r.resp.StatusCode < 500
}

// IsServerError returns true if status is a server error (5xx).
func (r *TestResponse) IsServerError() bool {
	return r.resp.StatusCode >= 500
}
