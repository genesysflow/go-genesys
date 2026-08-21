package middleware

import (
	"slices"

	"github.com/genesysflow/go-genesys/crypt"
	"github.com/genesysflow/go-genesys/http"
	"github.com/valyala/fasthttp"
)

// EncryptCookies encrypts every response cookie and decrypts incoming
// ones - Laravel's EncryptCookies middleware. Cookies the client cannot
// be allowed to read or forge (preferences, flags, identifiers) become
// opaque ciphertext in the browser while handlers keep working with
// plaintext values.
//
// Cookies named in except are passed through untouched - use it for
// cookies read by JavaScript or set by other systems. The session,
// CSRF, and remember-me cookies protect themselves (random ids, HMACs,
// hashed tokens) and may be excluded to save overhead:
//
//	kernel.Use(middleware.EncryptCookies(encrypter, "genesys_session", "genesys_csrf"))
//
// An incoming cookie that fails to decrypt is dropped, so handlers
// never see forged plaintext.
func EncryptCookies(encrypter *crypt.Encrypter, except ...string) http.MiddlewareFunc {
	return func(ctx *http.Context, next func() error) error {
		reqHeader := &ctx.FiberCtx().Request().Header

		// Decrypt incoming cookies in place; drop what will not decrypt.
		type pair struct{ key, value string }
		var rewrites []pair
		var drops []string
		reqHeader.VisitAllCookie(func(key, value []byte) {
			name := string(key)
			if slices.Contains(except, name) {
				return
			}
			plain, err := encrypter.DecryptString(string(value))
			if err != nil {
				drops = append(drops, name)
				return
			}
			rewrites = append(rewrites, pair{key: name, value: plain})
		})
		for _, p := range rewrites {
			reqHeader.SetCookie(p.key, p.value)
		}
		for _, name := range drops {
			reqHeader.DelCookie(name)
		}

		err := next()

		// Encrypt outgoing cookies set by handlers and later middleware.
		respHeader := &ctx.FiberCtx().Response().Header
		var outgoing []*fasthttp.Cookie
		respHeader.VisitAllCookie(func(key, value []byte) {
			if slices.Contains(except, string(key)) {
				return
			}
			cookie := fasthttp.AcquireCookie()
			if parseErr := cookie.ParseBytes(value); parseErr != nil {
				fasthttp.ReleaseCookie(cookie)
				return
			}
			outgoing = append(outgoing, cookie)
		})
		for _, cookie := range outgoing {
			ciphertext, encErr := encrypter.EncryptString(string(cookie.Value()))
			if encErr == nil {
				cookie.SetValue(ciphertext)
				respHeader.SetCookie(cookie)
			}
			fasthttp.ReleaseCookie(cookie)
		}

		return err
	}
}
