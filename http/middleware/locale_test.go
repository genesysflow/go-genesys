package middleware_test

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/genesysflow/go-genesys/foundation"
	genhttp "github.com/genesysflow/go-genesys/http"
	"github.com/genesysflow/go-genesys/http/middleware"
	"github.com/genesysflow/go-genesys/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func localeKernel(t *testing.T) *genhttp.Kernel {
	t.Helper()
	app := foundation.New()
	require.NoError(t, app.Boot())
	kernel := genhttp.NewKernel(app, genhttp.KernelConfig{DisableStartupMessage: true})

	sessions := session.NewManager(session.Config{CookieName: "loc_session", CookieSecure: false})
	kernel.Fiber().Use(sessions.Middleware())
	kernel.Use(middleware.DetectLocale(middleware.LocaleConfig{
		Supported: []string{"en", "de", "fr"},
	}))
	kernel.GET("/which", func(ctx *genhttp.Context) error {
		return ctx.String(middleware.LocaleFromContext(ctx))
	})
	return kernel
}

func request(t *testing.T, kernel *genhttp.Kernel, path string, headers map[string]string, cookies string) (string, []string) {
	t.Helper()
	req := httptest.NewRequest("GET", path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if cookies != "" {
		req.Header.Set("Cookie", cookies)
	}
	resp, err := kernel.Fiber().Test(req, -1)
	require.NoError(t, err)
	content, _ := io.ReadAll(resp.Body)
	var setCookies []string
	for _, c := range resp.Cookies() {
		setCookies = append(setCookies, c.Name+"="+c.Value)
	}
	return string(content), setCookies
}

func TestLocaleDetectionOrder(t *testing.T) {
	kernel := localeKernel(t)

	// Default: first supported locale.
	got, _ := request(t, kernel, "/which", nil, "")
	assert.Equal(t, "en", got)

	// Accept-Language, including region matching and q-values order.
	got, _ = request(t, kernel, "/which", map[string]string{"Accept-Language": "de-AT,de;q=0.9,en;q=0.8"}, "")
	assert.Equal(t, "de", got)

	// Unsupported languages fall through to the default.
	got, _ = request(t, kernel, "/which", map[string]string{"Accept-Language": "ja,ko;q=0.8"}, "")
	assert.Equal(t, "en", got)

	// Query param wins over the header.
	got, _ = request(t, kernel, "/which?locale=fr", map[string]string{"Accept-Language": "de"}, "")
	assert.Equal(t, "fr", got)

	// Unsupported query values are ignored.
	got, _ = request(t, kernel, "/which?locale=xx", map[string]string{"Accept-Language": "de"}, "")
	assert.Equal(t, "de", got)
}

func TestLocaleRememberedInSession(t *testing.T) {
	kernel := localeKernel(t)

	// Choose french explicitly; the session cookie comes back.
	_, cookies := request(t, kernel, "/which?locale=fr", nil, "")
	require.NotEmpty(t, cookies)

	// Subsequent plain requests with the session keep french, beating
	// the Accept-Language header.
	got, _ := request(t, kernel, "/which",
		map[string]string{"Accept-Language": "de"}, strings.Join(cookies, "; "))
	assert.Equal(t, "fr", got)
}
