package http_test

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	genesyserrors "github.com/genesysflow/go-genesys/errors"
	"github.com/genesysflow/go-genesys/foundation"
	genhttp "github.com/genesysflow/go-genesys/http"
	"github.com/genesysflow/go-genesys/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// postJSON posts a JSON body to a handler that validates T.
func postJSON(t *testing.T, k *genhttp.Kernel, path, body string) (int, map[string]any) {
	t.Helper()

	req := httptest.NewRequest("POST", path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := k.Fiber().Test(req, -1)
	require.NoError(t, err)

	var payload map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&payload)
	return resp.StatusCode, payload
}

func interfaceKernel(t *testing.T, register func(k *genhttp.Kernel)) *genhttp.Kernel {
	t.Helper()

	app := foundation.New()
	require.NoError(t, app.Register(&providers.AppServiceProvider{}))
	require.NoError(t, app.Register(&providers.ValidationServiceProvider{}))
	require.NoError(t, app.Boot())

	k := genhttp.NewKernel(app, genhttp.KernelConfig{DisableStartupMessage: true})
	register(k)
	return k
}

// --- Authorize -------------------------------------------------------

type adminOnlyRequest struct {
	Title string `json:"title" validate:"required"`
}

func (r *adminOnlyRequest) Authorize(ctx *genhttp.Context) bool {
	return ctx.Request().Header("X-Role") == "admin"
}

func TestFormRequestAuthorizeDeniesWith403(t *testing.T) {
	k := interfaceKernel(t, func(k *genhttp.Kernel) {
		k.POST("/posts", func(ctx *genhttp.Context) error {
			req, err := genhttp.ValidateRequest[adminOnlyRequest](ctx)
			if err != nil {
				return err
			}
			return ctx.JSONResponse(map[string]any{"title": req.Title})
		})
	})

	status, _ := postJSON(t, k, "/posts", `{"title":"Hello"}`)
	assert.Equal(t, 403, status)
}

func TestFormRequestAuthorizeAllows(t *testing.T) {
	k := interfaceKernel(t, func(k *genhttp.Kernel) {
		k.POST("/posts", func(ctx *genhttp.Context) error {
			req, err := genhttp.ValidateRequest[adminOnlyRequest](ctx)
			if err != nil {
				return err
			}
			return ctx.JSONResponse(map[string]any{"title": req.Title})
		})
	})

	body := strings.NewReader(`{"title":"Hello"}`)
	req := httptest.NewRequest("POST", "/posts", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Role", "admin")

	resp, err := k.Fiber().Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

// Authorization is checked before the rules run, so an unauthorized
// request with an invalid payload is a 403, never a 422 that leaks which
// fields exist.
func TestFormRequestAuthorizeRunsBeforeValidation(t *testing.T) {
	k := interfaceKernel(t, func(k *genhttp.Kernel) {
		k.POST("/posts", func(ctx *genhttp.Context) error {
			_, err := genhttp.ValidateRequest[adminOnlyRequest](ctx)
			return err
		})
	})

	status, _ := postJSON(t, k, "/posts", `{}`)
	assert.Equal(t, 403, status)
}

// --- PrepareForValidation --------------------------------------------

type slugRequest struct {
	Title string `json:"title" validate:"required"`
	Slug  string `json:"slug" validate:"required"`
}

func (r *slugRequest) PrepareForValidation(ctx *genhttp.Context) error {
	if r.Slug == "" {
		r.Slug = strings.ToLower(strings.ReplaceAll(r.Title, " ", "-"))
	}
	return nil
}

func TestFormRequestPrepareForValidationRunsBeforeRules(t *testing.T) {
	k := interfaceKernel(t, func(k *genhttp.Kernel) {
		k.POST("/posts", func(ctx *genhttp.Context) error {
			req, err := genhttp.ValidateRequest[slugRequest](ctx)
			if err != nil {
				return err
			}
			return ctx.JSONResponse(map[string]any{"slug": req.Slug})
		})
	})

	status, payload := postJSON(t, k, "/posts", `{"title":"Hello World"}`)
	require.Equal(t, 200, status)
	assert.Equal(t, "hello-world", payload["slug"])
}

type failingPrepareRequest struct {
	Title string `json:"title"`
}

func (r *failingPrepareRequest) PrepareForValidation(ctx *genhttp.Context) error {
	return genesyserrors.BadRequest("The payload could not be prepared.")
}

// An error from PrepareForValidation stops the request there.
func TestFormRequestPrepareErrorPropagates(t *testing.T) {
	k := interfaceKernel(t, func(k *genhttp.Kernel) {
		k.POST("/posts", func(ctx *genhttp.Context) error {
			_, err := genhttp.ValidateRequest[failingPrepareRequest](ctx)
			return err
		})
	})

	status, _ := postJSON(t, k, "/posts", `{"title":"Hello"}`)
	assert.Equal(t, 400, status)
}

// --- Messages and Attributes -----------------------------------------

type customMessageRequest struct {
	Email string `json:"email" validate:"required,email"`
}

func (r *customMessageRequest) Messages() map[string]string {
	return map[string]string{"email.required": "We need your email address."}
}

func TestFormRequestCustomMessages(t *testing.T) {
	k := interfaceKernel(t, func(k *genhttp.Kernel) {
		k.POST("/subscribe", func(ctx *genhttp.Context) error {
			_, err := genhttp.ValidateRequest[customMessageRequest](ctx)
			return err
		})
	})

	status, payload := postJSON(t, k, "/subscribe", `{}`)
	require.Equal(t, 422, status)

	errorsFor := payload["errors"].(map[string]any)["email"].([]any)
	assert.Contains(t, errorsFor, "We need your email address.")
}

type customAttributeRequest struct {
	EmailAddr string `json:"email_addr" validate:"required"`
}

func (r *customAttributeRequest) Attributes() map[string]string {
	return map[string]string{"email_addr": "email address"}
}

func TestFormRequestCustomAttributeNames(t *testing.T) {
	k := interfaceKernel(t, func(k *genhttp.Kernel) {
		k.POST("/subscribe", func(ctx *genhttp.Context) error {
			_, err := genhttp.ValidateRequest[customAttributeRequest](ctx)
			return err
		})
	})

	status, payload := postJSON(t, k, "/subscribe", `{}`)
	require.Equal(t, 422, status)

	messages := payload["errors"].(map[string]any)["email_addr"].([]any)
	require.NotEmpty(t, messages)
	assert.Contains(t, messages[0].(string), "email address")
}

// Per-request messages must not bleed into other requests through the
// shared validator instance.
func TestFormRequestCustomMessagesDoNotLeak(t *testing.T) {
	k := interfaceKernel(t, func(k *genhttp.Kernel) {
		k.POST("/subscribe", func(ctx *genhttp.Context) error {
			_, err := genhttp.ValidateRequest[customMessageRequest](ctx)
			return err
		})
		k.POST("/plain", func(ctx *genhttp.Context) error {
			_, err := genhttp.ValidateRequest[plainEmailRequest](ctx)
			return err
		})
	})

	status, _ := postJSON(t, k, "/subscribe", `{}`)
	require.Equal(t, 422, status)

	status, payload := postJSON(t, k, "/plain", `{}`)
	require.Equal(t, 422, status)
	messages := payload["errors"].(map[string]any)["email"].([]any)
	require.NotEmpty(t, messages)
	assert.NotContains(t, messages, "We need your email address.")
}

type plainEmailRequest struct {
	Email string `json:"email" validate:"required,email"`
}

// --- Rules -----------------------------------------------------------

type dynamicRulesRequest struct {
	Kind string `json:"kind" validate:"required"`
}

func (r *dynamicRulesRequest) Rules() map[string]string {
	// Company registrations must carry a VAT number; individuals need not.
	if r.Kind == "company" {
		return map[string]string{"vat": "required"}
	}
	return nil
}

func TestFormRequestDynamicRules(t *testing.T) {
	k := interfaceKernel(t, func(k *genhttp.Kernel) {
		k.POST("/register", func(ctx *genhttp.Context) error {
			_, err := genhttp.ValidateRequest[dynamicRulesRequest](ctx)
			if err != nil {
				return err
			}
			return ctx.String("ok")
		})
	})

	status, payload := postJSON(t, k, "/register", `{"kind":"company"}`)
	require.Equal(t, 422, status)
	assert.Contains(t, payload["errors"], "vat")

	status, _ = postJSON(t, k, "/register", `{"kind":"individual"}`)
	assert.Equal(t, 200, status)

	status, _ = postJSON(t, k, "/register", `{"kind":"company","vat":"GB123"}`)
	assert.Equal(t, 200, status)
}

// --- AfterValidation -------------------------------------------------

type afterHookRequest struct {
	Start int `json:"start" validate:"required"`
	End   int `json:"end" validate:"required"`
}

func (r *afterHookRequest) AfterValidation(ctx *genhttp.Context) map[string][]string {
	if r.End <= r.Start {
		return map[string][]string{"end": {"The end must come after the start."}}
	}
	return nil
}

func TestFormRequestAfterValidationHook(t *testing.T) {
	k := interfaceKernel(t, func(k *genhttp.Kernel) {
		k.POST("/ranges", func(ctx *genhttp.Context) error {
			_, err := genhttp.ValidateRequest[afterHookRequest](ctx)
			if err != nil {
				return err
			}
			return ctx.String("ok")
		})
	})

	status, payload := postJSON(t, k, "/ranges", `{"start":5,"end":2}`)
	require.Equal(t, 422, status)
	messages := payload["errors"].(map[string]any)["end"].([]any)
	assert.Contains(t, messages, "The end must come after the start.")

	status, _ = postJSON(t, k, "/ranges", `{"start":1,"end":9}`)
	assert.Equal(t, 200, status)
}

// The after hook only runs once the rules pass, matching Laravel: it
// would otherwise report on values the rules already rejected.
func TestFormRequestAfterValidationSkippedWhenRulesFail(t *testing.T) {
	k := interfaceKernel(t, func(k *genhttp.Kernel) {
		k.POST("/ranges", func(ctx *genhttp.Context) error {
			_, err := genhttp.ValidateRequest[afterHookRequest](ctx)
			return err
		})
	})

	status, payload := postJSON(t, k, "/ranges", `{}`)
	require.Equal(t, 422, status)

	fields := payload["errors"].(map[string]any)
	assert.Contains(t, fields, "start")
	if messages, ok := fields["end"].([]any); ok {
		assert.NotContains(t, messages, "The end must come after the start.")
	}
}

// A request implementing none of the interfaces behaves exactly as before.
func TestFormRequestWithoutInterfacesUnchanged(t *testing.T) {
	k := formKernel(t)

	status, _ := postJSON(t, k, "/users", `{"name":"Alice","email":"alice@example.com"}`)
	assert.Equal(t, 201, status)
}

// preparedSlugRequest fills in a value the form left blank, then names a
// rule for it. Laravel's prepareForValidation merges into the input the
// rules see, so a derived value is checked, not skipped.
type preparedSlugRequest struct {
	Title string `json:"title" form:"title" validate:"required"`
	Slug  string `json:"slug" form:"slug"`
}

func (r *preparedSlugRequest) PrepareForValidation(ctx *genhttp.Context) error {
	if r.Slug == "" {
		r.Slug = strings.ToLower(strings.ReplaceAll(r.Title, " ", "-"))
	}
	return nil
}

func (r *preparedSlugRequest) Rules() map[string]string {
	return map[string]string{"slug": "required,max=8"}
}

func TestRulesSeeValuesPrepareForValidationDerived(t *testing.T) {
	kernel := interfaceKernel(t, func(k *genhttp.Kernel) {
		k.POST("/prepared", func(ctx *genhttp.Context) error {
			req, err := genhttp.ValidateRequest[preparedSlugRequest](ctx)
			if err != nil {
				return err
			}
			return ctx.JSONResponse(map[string]any{"slug": req.Slug})
		})
	})

	// The derived slug is short enough.
	status, payload := postJSON(t, kernel, "/prepared", `{"title":"Ada Bee"}`)
	assert.Equal(t, 200, status)
	assert.Equal(t, "ada-bee", payload["slug"])

	// And too long a derived slug is caught, rather than slipping past
	// because the form never sent the field.
	status, payload = postJSON(t, kernel, "/prepared", `{"title":"A Much Longer Title"}`)
	require.Equal(t, 422, status)
	errs, _ := payload["errors"].(map[string]any)
	assert.Contains(t, errs, "slug")
}

// Input the struct does not carry is still available to Rules(), so a
// rule can name a field the form request does not bind.
func TestRulesStillSeeUnboundInput(t *testing.T) {
	kernel := interfaceKernel(t, func(k *genhttp.Kernel) {
		k.POST("/unbound", func(ctx *genhttp.Context) error {
			_, err := genhttp.ValidateRequest[unboundRulesRequest](ctx)
			if err != nil {
				return err
			}
			return ctx.String("ok")
		})
	})

	status, payload := postJSON(t, kernel, "/unbound", `{"title":"Ada","captcha":"no"}`)
	require.Equal(t, 422, status)
	errs, _ := payload["errors"].(map[string]any)
	assert.Contains(t, errs, "captcha")
}

type unboundRulesRequest struct {
	Title string `json:"title" validate:"required"`
}

func (r *unboundRulesRequest) Rules() map[string]string {
	return map[string]string{"captcha": "eq=yes"}
}
