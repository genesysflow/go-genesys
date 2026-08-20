package middleware

import (
	"strings"

	"github.com/genesysflow/go-genesys/http"
	"github.com/genesysflow/go-genesys/session"
)

// LocaleConfig configures locale detection.
type LocaleConfig struct {
	// Supported are the locales the app serves; the first is the
	// default. Required.
	Supported []string

	// QueryParam checked first (default "locale"); a matching value is
	// remembered in the session when one is present.
	QueryParam string

	// SessionKey remembers the visitor's choice (default "_locale").
	SessionKey string

	// OnDetect, when set, is called with the resolved locale - wire it
	// to translator.In(...) storage or logging.
	OnDetect func(ctx *http.Context, locale string)
}

const localeContextKey = "_detected_locale"

// DetectLocale resolves the request locale from (in order) an explicit
// query parameter, the session, and the Accept-Language header, always
// constrained to the supported list. The result lands in the request
// context; read it with LocaleFromContext:
//
//	kernel.Use(middleware.DetectLocale(middleware.LocaleConfig{
//	    Supported: []string{"en", "de", "fr"},
//	}))
//
//	// in a handler:
//	view := translator.In(middleware.LocaleFromContext(ctx))
func DetectLocale(config LocaleConfig) http.MiddlewareFunc {
	if config.QueryParam == "" {
		config.QueryParam = "locale"
	}
	if config.SessionKey == "" {
		config.SessionKey = "_locale"
	}
	supported := make(map[string]bool, len(config.Supported))
	for _, locale := range config.Supported {
		supported[locale] = true
	}
	fallback := ""
	if len(config.Supported) > 0 {
		fallback = config.Supported[0]
	}

	return func(ctx *http.Context, next func() error) error {
		locale := ""
		sess := session.GetFromContext(ctx.FiberCtx())

		// 1. Explicit choice via query param - remembered when possible.
		if requested := ctx.Query(config.QueryParam); requested != "" && supported[requested] {
			locale = requested
			if sess != nil {
				sess.Set(config.SessionKey, requested)
			}
		}

		// 2. Previously remembered choice.
		if locale == "" && sess != nil {
			if remembered := sess.GetString(config.SessionKey); supported[remembered] {
				locale = remembered
			}
		}

		// 3. Accept-Language negotiation.
		if locale == "" {
			locale = negotiateLocale(ctx.Request().Header("Accept-Language"), supported)
		}
		if locale == "" {
			locale = fallback
		}

		ctx.Set(localeContextKey, locale)
		if config.OnDetect != nil {
			config.OnDetect(ctx, locale)
		}
		return next()
	}
}

// LocaleFromContext returns the locale resolved by DetectLocale, or ""
// when the middleware did not run.
func LocaleFromContext(ctx *http.Context) string {
	if locale, ok := ctx.Get(localeContextKey).(string); ok {
		return locale
	}
	return ""
}

// negotiateLocale picks the first supported language from an
// Accept-Language header, honouring order and matching "de-AT" to "de".
func negotiateLocale(header string, supported map[string]bool) string {
	for _, part := range strings.Split(header, ",") {
		tag := strings.TrimSpace(strings.SplitN(part, ";", 2)[0])
		if tag == "" {
			continue
		}
		if supported[tag] {
			return tag
		}
		if base, _, found := strings.Cut(tag, "-"); found && supported[base] {
			return base
		}
	}
	return ""
}
