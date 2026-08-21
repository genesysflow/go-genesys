package middleware

import (
	"github.com/genesysflow/go-genesys/errors"
	"github.com/genesysflow/go-genesys/http"
	"github.com/genesysflow/go-genesys/urlsign"
)

// ValidateSignature rejects requests whose URL signature is missing,
// invalid, or expired (403). Pair with urlsign.Signer for issuing links:
//
//	signer := urlsign.New(key)
//	kernel.GET("/unsubscribe", handler, middleware.ValidateSignature(key))
func ValidateSignature(key []byte) http.MiddlewareFunc {
	signer := urlsign.New(key)
	return func(ctx *http.Context, next func() error) error {
		target := ctx.Path()
		if query := string(ctx.FiberCtx().Request().URI().QueryString()); query != "" {
			target += "?" + query
		}
		if !signer.Verify(target) {
			return errors.Forbidden("Invalid signature.")
		}
		return next()
	}
}
