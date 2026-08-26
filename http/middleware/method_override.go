package middleware

import (
	"strings"

	"github.com/genesysflow/go-genesys/view"
	"github.com/gofiber/fiber/v2"
)

// DefaultMethodField is the form field an HTML form declares its real
// verb in, and what the method_field template helper writes.
const DefaultMethodField = "_method"

// DefaultMethodHeader is the header API clients use for the same
// purpose, for the benefit of proxies that refuse the verb outright.
const DefaultMethodHeader = "X-HTTP-Method-Override"

// MethodOverrideConfig configures the method override middleware.
type MethodOverrideConfig struct {
	// FieldName is the form field carrying the declared verb (default
	// "_method").
	FieldName string

	// HeaderName is the header carrying the declared verb (default
	// "X-HTTP-Method-Override").
	HeaderName string

	// DisableHeader stops the header from being honoured, leaving the
	// form field as the only way to declare a verb.
	//
	// It is phrased negatively because the header grants nothing the
	// field does not: it obeys the same allowlist and the same POST-only
	// rule, and it is harder for another site to set, not easier - a
	// cross-origin form can put any field in a body it submits, but it
	// cannot add a header without the preflight the browser will refuse.
	// The option is here for operators who would rather the surface did
	// not exist at all.
	DisableHeader bool
}

// MethodOverride lets a request declare a verb its sender could not
// otherwise send, honouring the _method field the method_field helper
// writes. An HTML form can only be submitted as GET or POST, so without
// this a PUT or DELETE route is reachable by scripts and unreachable by
// the pages the application renders.
//
// It is a Fiber handler rather than an http.MiddlewareFunc because those
// run inside the handler the router already chose: by then a POST that
// meant DELETE has either matched the wrong route or been answered 405,
// and there is nothing left to correct. The verb has to be settled
// before the router looks at it, which is what running here achieves -
// and it means every MiddlewareFunc, the CSRF check included, sees the
// verb that will actually be dispatched.
//
// The rules are narrow on purpose, because this is a request-forging
// surface:
//
//   - Only a POST is rewritten. A GET carrying the field is left alone,
//     so a link, a prefetch or a crawler can never delete anything.
//   - Only to PUT, PATCH or DELETE, the verbs view.SpoofedMethod allows
//     a form to declare. Anything else proceeds as the POST it was.
//   - The field is read from the request body only. A verb in the query
//     string is ignored, so it cannot be smuggled in a URL that something
//     else - a redirect, a referrer, a log - will happily carry around.
func MethodOverride(config ...MethodOverrideConfig) fiber.Handler {
	cfg := MethodOverrideConfig{}
	if len(config) > 0 {
		cfg = config[0]
	}
	if cfg.FieldName == "" {
		cfg.FieldName = DefaultMethodField
	}
	if cfg.HeaderName == "" {
		cfg.HeaderName = DefaultMethodHeader
	}

	return func(c *fiber.Ctx) error {
		// A verb is only ever spoofed upwards, from the POST a form was
		// able to send. Rewriting anything else would let a request that
		// needed no user intent at all turn into one that changes state.
		if c.Method() != fiber.MethodPost {
			return c.Next()
		}

		declared := declaredMethod(c, cfg)
		if declared == "" {
			return c.Next()
		}

		if verb, ok := view.SpoofedMethod(declared); ok {
			c.Method(verb)
		}

		return c.Next()
	}
}

// declaredMethod reads the verb a request declared, from the body or the
// header but never from the query string.
//
// Fiber's FormValue cannot be used here: it consults the query arguments
// first, which is precisely the source this must not trust.
func declaredMethod(c *fiber.Ctx, cfg MethodOverrideConfig) string {
	if value := c.Request().PostArgs().Peek(cfg.FieldName); len(value) > 0 {
		return string(value)
	}

	// A form that uploads a file is multipart, and its fields are not in
	// PostArgs. Parsing it costs the body, which is why it is only
	// reached for the content type that requires it; a body that will not
	// parse is simply a request with no declared verb.
	if isMultipart(c) {
		if form, err := c.MultipartForm(); err == nil {
			if values := form.Value[cfg.FieldName]; len(values) > 0 {
				return values[0]
			}
		}
	}

	if !cfg.DisableHeader {
		return c.Get(cfg.HeaderName)
	}

	return ""
}

// isMultipart reports whether the body is a multipart form, the only
// case in which parsing one is worth the cost.
func isMultipart(c *fiber.Ctx) bool {
	return strings.HasPrefix(
		strings.ToLower(strings.TrimSpace(string(c.Request().Header.ContentType()))),
		fiber.MIMEMultipartForm,
	)
}
