// Package http provides HTTP handling built on top of Fiber.
package http

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"mime/multipart"
	"strconv"
	"strings"
	"sync"

	"github.com/genesysflow/go-genesys/contracts"
	"github.com/gofiber/fiber/v2"
)

// Request wraps Fiber's request to provide a Laravel-like API.
type Request struct {
	ctx   *fiber.Ctx
	store sync.Map

	// jsonDecoded caches the parsed JSON body. Input() reads one key at
	// a time and All() reads every key, so without this a request with
	// n fields unmarshals its body n times.
	jsonDecoded map[string]any
	jsonParsed  bool
}

// NewRequest creates a new Request wrapper.
func NewRequest(ctx *fiber.Ctx) *Request {
	return &Request{
		ctx: ctx,
	}
}

// FiberCtx returns the underlying Fiber context.
func (r *Request) FiberCtx() *fiber.Ctx {
	return r.ctx
}

// Context returns the request context.
func (r *Request) Context() context.Context {
	return r.ctx.UserContext()
}

// WithContext returns a copy of the request with the given context.
func (r *Request) WithContext(ctx context.Context) contracts.Request {
	r.ctx.SetUserContext(ctx)
	return r
}

// Method returns the HTTP method.
func (r *Request) Method() string {
	return r.ctx.Method()
}

// URI returns the request URI.
func (r *Request) URI() string {
	return r.ctx.OriginalURL()
}

// Path returns the request path.
func (r *Request) Path() string {
	return r.ctx.Path()
}

// FullURL returns the full URL including query string.
func (r *Request) FullURL() string {
	return r.Scheme() + "://" + r.Host() + r.ctx.OriginalURL()
}

// Host returns the host.
func (r *Request) Host() string {
	return r.ctx.Hostname()
}

// Scheme returns the scheme (http or https).
func (r *Request) Scheme() string {
	if r.ctx.Protocol() == "https" {
		return "https"
	}
	return "http"
}

// IsSecure returns true if the request is using HTTPS.
func (r *Request) IsSecure() bool {
	return r.ctx.Protocol() == "https"
}

// IP returns the client IP address.
func (r *Request) IP() string {
	return r.ctx.IP()
}

// IPs returns all client IP addresses from X-Forwarded-For.
func (r *Request) IPs() []string {
	return r.ctx.IPs()
}

// Header returns a header value.
func (r *Request) Header(key string) string {
	return r.ctx.Get(key)
}

// Headers returns all headers.
func (r *Request) Headers() map[string]string {
	headers := make(map[string]string)
	for key, value := range r.ctx.Request().Header.All() {
		headers[string(key)] = string(value)
	}
	return headers
}

// Query returns a query string parameter.
func (r *Request) Query(key string, defaultValue ...string) string {
	value := r.ctx.Query(key)
	if value == "" && len(defaultValue) > 0 {
		return defaultValue[0]
	}
	return value
}

// QueryInt returns a query parameter as integer.
func (r *Request) QueryInt(key string, defaultValue ...int) int {
	value := r.ctx.Query(key)
	if value == "" {
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return 0
	}
	intValue, err := strconv.Atoi(value)
	if err != nil {
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return 0
	}
	return intValue
}

// Param returns a route parameter.
func (r *Request) Param(key string, defaultValue ...string) string {
	value := r.ctx.Params(key)
	if value == "" && len(defaultValue) > 0 {
		return defaultValue[0]
	}
	return value
}

// ParamInt returns a route parameter as integer.
func (r *Request) ParamInt(key string, defaultValue ...int) int {
	value := r.ctx.Params(key)
	if value == "" {
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return 0
	}
	intValue, err := strconv.Atoi(value)
	if err != nil {
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return 0
	}
	return intValue
}

// Input returns a request input value (from body, query, or params).
func (r *Request) Input(key string, defaultValue ...string) string {
	// Check route params first
	if value := r.ctx.Params(key); value != "" {
		return value
	}

	// Check query params
	if value := r.ctx.Query(key); value != "" {
		return value
	}

	// Check form data
	if value := r.ctx.FormValue(key); value != "" {
		return value
	}

	// Check a JSON body: for an API request it holds all the input there
	// is, so skipping it would report every field as absent.
	if value, ok := r.jsonBody()[key]; ok {
		if str := stringifyInput(value); str != "" {
			return str
		}
	}

	// Default value
	if len(defaultValue) > 0 {
		return defaultValue[0]
	}
	return ""
}

// jsonBody decodes a JSON object body into a map. It returns nil for any
// other content type, an empty body, or a body that is not an object (a
// bare array or scalar is a payload, not named input).
func (r *Request) jsonBody() map[string]any {
	// Parsed once per request. A nil result is cached too, so a body
	// that is not a JSON object is not re-parsed on every lookup.
	if r.jsonParsed {
		return r.jsonDecoded
	}
	r.jsonParsed = true

	if !r.IsJSON() {
		return nil
	}

	body := r.ctx.Body()
	if len(body) == 0 {
		return nil
	}

	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil
	}

	r.jsonDecoded = decoded
	return decoded
}

// stringifyInput renders a decoded JSON value as request input. Nested
// objects and arrays have no single string form, so they report empty and
// callers reach them through All().
func stringifyInput(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case bool:
		return strconv.FormatBool(typed)
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case json.Number:
		return typed.String()
	default:
		return ""
	}
}

// All returns all input data.
func (r *Request) All() map[string]any {
	data := make(map[string]any)

	// Add query params
	for key, value := range r.ctx.Request().URI().QueryArgs().All() {
		data[string(key)] = string(value)
	}

	// Add form data
	form, err := r.ctx.MultipartForm()
	if err == nil && form != nil {
		for key, values := range form.Value {
			if len(values) == 1 {
				data[key] = values[0]
			} else {
				data[key] = values
			}
		}
	} else {
		// Try regular form data
		for key, value := range r.ctx.Request().PostArgs().All() {
			data[string(key)] = string(value)
		}
	}

	// Add a JSON body, which for an API request is the whole of the input.
	for key, value := range r.jsonBody() {
		data[key] = value
	}

	// Add route params
	for _, param := range r.ctx.Route().Params {
		data[param] = r.ctx.Params(param)
	}

	return data
}

// Only returns only the specified input keys.
func (r *Request) Only(keys ...string) map[string]any {
	all := r.All()
	result := make(map[string]any, len(keys))
	for _, key := range keys {
		if value, ok := all[key]; ok {
			result[key] = value
		}
	}
	return result
}

// Except returns all input except the specified keys.
func (r *Request) Except(keys ...string) map[string]any {
	all := r.All()
	keySet := make(map[string]bool, len(keys))
	for _, key := range keys {
		keySet[key] = true
	}
	result := make(map[string]any)
	for key, value := range all {
		if !keySet[key] {
			result[key] = value
		}
	}
	return result
}

// Has checks if an input key exists.
func (r *Request) Has(key string) bool {
	return r.Input(key) != ""
}

// Filled checks if an input key exists and is not empty.
func (r *Request) Filled(key string) bool {
	return strings.TrimSpace(r.Input(key)) != ""
}

// Body returns the raw request body.
func (r *Request) Body() []byte {
	return r.ctx.Body()
}

// BodyReader returns an io.Reader for the request body.
func (r *Request) BodyReader() io.Reader {
	return bytes.NewReader(r.ctx.Body())
}

// JSON parses the JSON body into the given struct.
func (r *Request) JSON(v any) error {
	return r.ctx.BodyParser(v)
}

// File returns an uploaded file.
func (r *Request) File(key string) (*multipart.FileHeader, error) {
	return r.ctx.FormFile(key)
}

// Files returns all uploaded files for a key.
func (r *Request) Files(key string) ([]*multipart.FileHeader, error) {
	form, err := r.ctx.MultipartForm()
	if err != nil {
		return nil, err
	}
	if form == nil || form.File == nil {
		return nil, nil
	}
	return form.File[key], nil
}

// Cookie returns a cookie value.
func (r *Request) Cookie(key string) string {
	return r.ctx.Cookies(key)
}

// Cookies returns all cookies.
func (r *Request) Cookies() map[string]string {
	cookies := make(map[string]string)
	for key, value := range r.ctx.Request().Header.Cookies() {
		cookies[string(key)] = string(value)
	}
	return cookies
}

// IsAjax returns true if this is an AJAX request.
func (r *Request) IsAjax() bool {
	return r.ctx.XHR()
}

// IsJSON returns true if the request wants JSON response.
func (r *Request) IsJSON() bool {
	accept := r.ctx.Accepts("application/json", "text/html")
	return accept == "application/json"
}

// Accepts checks if the request accepts the given content type.
func (r *Request) Accepts(contentType string) bool {
	return r.ctx.Accepts(contentType) == contentType
}

// Get retrieves a value from the request store.
func (r *Request) Get(key string) any {
	value, _ := r.store.Load(key)
	return value
}

// Set stores a value in the request store.
func (r *Request) Set(key string, value any) {
	r.store.Store(key, value)
}

// ContentType returns the Content-Type header.
func (r *Request) ContentType() string {
	return string(r.ctx.Request().Header.ContentType())
}

// UserAgent returns the User-Agent header.
func (r *Request) UserAgent() string {
	return r.ctx.Get("User-Agent")
}

// Referer returns the Referer header.
func (r *Request) Referer() string {
	return r.ctx.Get("Referer")
}

// BearerToken extracts the bearer token from the Authorization header.
// The scheme is matched case-insensitively, as RFC 7235 requires.
func (r *Request) BearerToken() string {
	scheme, token, found := strings.Cut(r.ctx.Get("Authorization"), " ")
	if !found || !strings.EqualFold(scheme, "Bearer") {
		return ""
	}
	return strings.TrimSpace(token)
}

// BasicAuth extracts and decodes HTTP Basic credentials from the Authorization
// header, per RFC 7617.
//
// ok reports only that a well-formed Basic header was present and decoded; it
// makes no claim about the credentials being valid. Callers must verify them,
// and should use crypto/subtle.ConstantTimeCompare to do so.
func (r *Request) BasicAuth() (username, password string, ok bool) {
	scheme, encoded, found := strings.Cut(r.ctx.Get("Authorization"), " ")
	if !found || !strings.EqualFold(scheme, "Basic") {
		return "", "", false
	}

	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		return "", "", false
	}

	// The separator is the first colon: a password may itself contain colons,
	// a username may not.
	username, password, found = strings.Cut(string(decoded), ":")
	if !found {
		return "", "", false
	}

	return username, password, true
}
