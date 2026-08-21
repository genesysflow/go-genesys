package middleware_test

import (
	"io"
	nethttp "net/http"
	"net/http/httptest"
	"testing"

	"github.com/genesysflow/go-genesys/crypt"
	genhttp "github.com/genesysflow/go-genesys/http"
	"github.com/genesysflow/go-genesys/http/middleware"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func encTestKernel(t *testing.T) (*genhttp.Kernel, *crypt.Encrypter) {
	t.Helper()
	enc, err := crypt.New([]byte("0123456789abcdef0123456789abcdef"))
	require.NoError(t, err)
	k := newKernel(t)
	k.Use(middleware.EncryptCookies(enc, "plain_cookie"))
	return k, enc
}

func TestEncryptCookiesRoundTrip(t *testing.T) {
	k, enc := encTestKernel(t)

	k.GET("/set", func(ctx *genhttp.Context) error {
		ctx.FiberCtx().Cookie(fiberCookie("prefs", "dark-mode"))
		return ctx.String("ok")
	})
	k.GET("/read", func(ctx *genhttp.Context) error {
		return ctx.String("value=" + ctx.FiberCtx().Cookies("prefs"))
	})

	resp, err := k.Fiber().Test(httptest.NewRequest("GET", "/set", nil), -1)
	require.NoError(t, err)

	var stored string
	for _, c := range resp.Cookies() {
		if c.Name == "prefs" {
			stored = c.Value
		}
	}
	require.NotEmpty(t, stored)
	assert.NotEqual(t, "dark-mode", stored, "the browser sees ciphertext")
	plain, err := enc.DecryptString(stored)
	require.NoError(t, err)
	assert.Equal(t, "dark-mode", plain)

	// Sending the ciphertext back, the handler reads plaintext.
	req := httptest.NewRequest("GET", "/read", nil)
	req.Header.Set("Cookie", "prefs="+stored)
	resp, err = k.Fiber().Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, "value=dark-mode", bodyOf(t, resp))
}

func TestEncryptCookiesDropsForgeries(t *testing.T) {
	k, _ := encTestKernel(t)
	k.GET("/read", func(ctx *genhttp.Context) error {
		return ctx.String("value=" + ctx.FiberCtx().Cookies("prefs", "<absent>"))
	})

	req := httptest.NewRequest("GET", "/read", nil)
	req.Header.Set("Cookie", "prefs=forged-plaintext")
	resp, err := k.Fiber().Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, "value=<absent>", bodyOf(t, resp), "a cookie that fails to decrypt never reaches the handler")
}

func TestEncryptCookiesHonoursExceptions(t *testing.T) {
	k, _ := encTestKernel(t)
	k.GET("/set", func(ctx *genhttp.Context) error {
		ctx.FiberCtx().Cookie(fiberCookie("plain_cookie", "readable"))
		return ctx.String("ok")
	})

	resp, err := k.Fiber().Test(httptest.NewRequest("GET", "/set", nil), -1)
	require.NoError(t, err)
	for _, c := range resp.Cookies() {
		if c.Name == "plain_cookie" {
			assert.Equal(t, "readable", c.Value, "excluded cookies pass through untouched")
			return
		}
	}
	t.Fatal("plain_cookie was not set")
}

// fiberCookie builds a fiber cookie value struct.
func fiberCookie(name, value string) *fiber.Cookie {
	return &fiber.Cookie{Name: name, Value: value, Path: "/"}
}

func bodyOf(t *testing.T, resp *nethttp.Response) string {
	t.Helper()
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	resp.Body.Close()
	return string(raw)
}
