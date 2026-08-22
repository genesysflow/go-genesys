package middleware

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/genesysflow/go-genesys/http"
	"github.com/gofiber/fiber/v2"
)

// CSRFContextKey is the context key under which the current CSRF token is
// stored. Templates should render it into forms as a hidden field, and into a
// <meta> tag for JavaScript clients to echo back in the request header.
const CSRFContextKey = "csrf_token"

// csrfSafeMethods are the methods RFC 9110 defines as safe. They are expected
// not to change server state, so they are not required to carry a token — but
// they do seed one for the subsequent unsafe request.
var csrfSafeMethods = []string{fiber.MethodGet, fiber.MethodHead, fiber.MethodOptions, fiber.MethodTrace}

// CSRFConfig configures the CSRF middleware.
type CSRFConfig struct {
	// Secret signs issued tokens. It MUST be set, and MUST be identical
	// across every instance of the application: a token issued by one replica
	// is validated by another, and a per-process random secret would reject
	// those tokens and break every form after a restart or a load-balancer
	// hop. Use at least 32 bytes from a secret manager or APP_KEY.
	Secret []byte

	// CookieName is the cookie carrying the token.
	CookieName string

	// HeaderName is the request header checked for the token, used by
	// JavaScript clients.
	HeaderName string

	// FieldName is the form field checked for the token, used by HTML forms.
	FieldName string

	CookiePath   string
	CookieDomain string

	// CookieInsecure drops the Secure attribute from the cookie. It is phrased
	// negatively so that the zero value is the safe one: a CSRF cookie sent
	// over plaintext HTTP can be read and replayed by anyone on the network
	// path, which defeats the control, so insecure transport is the thing that
	// must be opted into. Set it only for local development over http://.
	CookieInsecure bool

	CookieHTTPOnly bool
	CookieSameSite string

	// Expiration bounds how long an issued token stays valid.
	Expiration time.Duration

	// TrustedOrigins are additional origins ("https://app.example.com") whose
	// cross-origin requests are accepted. The request's own origin is always
	// trusted.
	TrustedOrigins []string

	// Except lists path prefixes exempt from verification, for endpoints
	// that do not authenticate with a cookie: a bearer-token API carries
	// no cookie, so it cannot carry a double-submit token either, and
	// there is no cross-site request to forge without one.
	//
	// Entries are prefixes, matched against the request path. Exempt only
	// what genuinely does not use cookies - an exempt path that does is an
	// unguarded one.
	Except []string

	// ErrorHandler renders the rejection. Defaults to a 403 JSON response.
	ErrorHandler func(ctx *http.Context) error
}

// DefaultCSRFConfig returns the default CSRF configuration.
//
// The cookie is marked Secure unless CookieInsecure is set; see that field.
//
// CookieHTTPOnly defaults to false because the double-submit pattern requires
// JavaScript to read the cookie and echo it back in HeaderName. Server-rendered
// forms that only use FieldName can and should set it to true.
func DefaultCSRFConfig() CSRFConfig {
	return CSRFConfig{
		CookieName:     "genesys_csrf",
		HeaderName:     "X-CSRF-Token",
		FieldName:      "_token",
		CookiePath:     "/",
		CookieHTTPOnly: false,
		CookieSameSite: "Lax",
		Expiration:     12 * time.Hour,
	}
}

// CSRF creates a Cross-Site Request Forgery protection middleware.
//
// It implements the signed double-submit cookie pattern. A random token is
// issued in a cookie; every state-changing request must echo that same token
// back out-of-band, in either a header or a form field. A cross-site attacker
// can cause the victim's browser to send the cookie, but the same-origin
// policy stops them from reading it, so they cannot produce the matching copy.
//
// Tokens are HMAC-signed so that a forged cookie — one injected by a sibling
// subdomain, which the same-origin policy does not prevent — is rejected
// rather than trivially satisfying "cookie equals form field".
//
// As defence in depth the Origin (or Referer) header is checked against the
// request host, which catches the same attack independently of the token.
//
// This middleware must run after any session middleware and before route
// handlers, and it must not be applied to stateless token-authenticated APIs,
// where it serves no purpose (no ambient credential to abuse).
func CSRF(config ...CSRFConfig) http.MiddlewareFunc {
	cfg := DefaultCSRFConfig()
	if len(config) > 0 {
		user := config[0]
		// Fill unset fields from the defaults so callers can override only
		// what they care about without silently disabling the cookie name.
		// Only fields whose zero value unambiguously means "unset" can be
		// merged this way; the bool fields are therefore phrased so that their
		// zero value is already the intended default (see CookieInsecure).
		if user.CookieName == "" {
			user.CookieName = cfg.CookieName
		}
		if user.HeaderName == "" {
			user.HeaderName = cfg.HeaderName
		}
		if user.FieldName == "" {
			user.FieldName = cfg.FieldName
		}
		if user.CookiePath == "" {
			user.CookiePath = cfg.CookiePath
		}
		if user.CookieSameSite == "" {
			user.CookieSameSite = cfg.CookieSameSite
		}
		if user.Expiration == 0 {
			user.Expiration = cfg.Expiration
		}
		cfg = user
	}

	if cfg.ErrorHandler == nil {
		cfg.ErrorHandler = func(ctx *http.Context) error {
			return ctx.Status(fiber.StatusForbidden).JSONResponse(fiber.Map{
				"error": "CSRF token mismatch",
			})
		}
	}

	// Refuse to run unconfigured rather than silently generating a
	// per-process secret. A random secret would appear to work in a
	// single-instance test and then reject valid tokens in production the
	// moment a second replica or a restart is involved — a failure that looks
	// like a flaky app rather than a misconfiguration.
	if len(cfg.Secret) == 0 {
		panic("middleware: CSRF requires a non-empty Secret shared across all application instances")
	}
	secret := slices.Clone(cfg.Secret)

	return func(ctx *http.Context, next func() error) error {
		// An exempt path still gets a token issued, so a form rendered by
		// one can post to a guarded path.
		if csrfExempt(ctx.Path(), cfg.Except) {
			ctx.Set(CSRFContextKey, csrfEnsureToken(ctx, cfg, secret))
			return next()
		}

		if slices.Contains(csrfSafeMethods, ctx.Method()) {
			token := csrfEnsureToken(ctx, cfg, secret)
			ctx.Set(CSRFContextKey, token)
			return next()
		}

		if !csrfOriginAllowed(ctx, cfg) {
			return cfg.ErrorHandler(ctx)
		}

		cookieToken := ctx.Request().Cookie(cfg.CookieName)
		sentToken := csrfSentToken(ctx, cfg)

		if cookieToken == "" || sentToken == "" {
			return cfg.ErrorHandler(ctx)
		}

		// The signature check is what makes the pair meaningful: without it,
		// an attacker who can set a cookie could choose both halves.
		if !csrfValidSignature(cookieToken, secret) {
			return cfg.ErrorHandler(ctx)
		}

		if subtle.ConstantTimeCompare([]byte(cookieToken), []byte(sentToken)) != 1 {
			return cfg.ErrorHandler(ctx)
		}

		ctx.Set(CSRFContextKey, cookieToken)

		return next()
	}
}

// csrfExempt reports whether a path is exempt from verification. The
// match is a prefix, not a substring: /evil/api/ must not inherit the
// exemption granted to /api/.
func csrfExempt(path string, except []string) bool {
	for _, prefix := range except {
		if prefix != "" && strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

// CSRFToken returns the token issued for this request, for rendering into
// forms and meta tags. It is empty if the CSRF middleware did not run.
func CSRFToken(ctx *http.Context) string {
	token, _ := ctx.Get(CSRFContextKey).(string)
	return token
}

// csrfEnsureToken returns the request's existing valid token, minting and
// setting a new one if absent or unverifiable.
func csrfEnsureToken(ctx *http.Context, cfg CSRFConfig, secret []byte) string {
	if existing := ctx.Request().Cookie(cfg.CookieName); existing != "" && csrfValidSignature(existing, secret) {
		return existing
	}

	token := csrfNewToken(secret)

	ctx.FiberCtx().Cookie(&fiber.Cookie{
		Name:     cfg.CookieName,
		Value:    token,
		Path:     cfg.CookiePath,
		Domain:   cfg.CookieDomain,
		Expires:  time.Now().Add(cfg.Expiration),
		Secure:   !cfg.CookieInsecure,
		HTTPOnly: cfg.CookieHTTPOnly,
		SameSite: cfg.CookieSameSite,
	})

	return token
}

// csrfNewToken mints "<random>.<hmac>" using 32 bytes of cryptographic
// randomness.
func csrfNewToken(secret []byte) string {
	raw := make([]byte, 32)
	// crypto/rand.Read never returns an error; it panics internally if the
	// system source fails, which is the correct outcome — proceeding with a
	// predictable token would be worse than crashing.
	_, _ = rand.Read(raw)

	value := base64.RawURLEncoding.EncodeToString(raw)

	return value + "." + csrfSign(value, secret)
}

// csrfSign returns the base64 HMAC-SHA256 of value.
func csrfSign(value string, secret []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(value))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// csrfValidSignature reports whether token carries a signature this server
// produced.
func csrfValidSignature(token string, secret []byte) bool {
	value, signature, found := strings.Cut(token, ".")
	if !found {
		return false
	}
	return hmac.Equal([]byte(signature), []byte(csrfSign(value, secret)))
}

// csrfSentToken reads the out-of-band copy of the token.
//
// The query string is deliberately not consulted: a token there would leak
// into access logs, browser history and Referer headers.
func csrfSentToken(ctx *http.Context, cfg CSRFConfig) string {
	if token := ctx.Request().Header(cfg.HeaderName); token != "" {
		return token
	}
	return ctx.FiberCtx().FormValue(cfg.FieldName)
}

// csrfOriginAllowed checks Origin, falling back to Referer, against the host
// the request was addressed to.
func csrfOriginAllowed(ctx *http.Context, cfg CSRFConfig) bool {
	origin := ctx.Request().Header("Origin")
	if origin == "" || origin == "null" {
		// Not every browser sends Origin on same-origin form posts; Referer is
		// the historical fallback.
		origin = ctx.Request().Header("Referer")
	}

	if origin == "" {
		// Neither header present. The token check still applies, so allow it
		// rather than breaking non-browser clients that legitimately hold a
		// token.
		return true
	}

	parsed, err := url.Parse(origin)
	if err != nil || parsed.Host == "" {
		return false
	}

	if strings.EqualFold(parsed.Host, ctx.Request().Host()) {
		return true
	}

	candidate := parsed.Scheme + "://" + parsed.Host
	for _, trusted := range cfg.TrustedOrigins {
		if strings.EqualFold(trusted, candidate) {
			return true
		}
	}

	return false
}
