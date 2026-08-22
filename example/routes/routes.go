package routes

import (
	"crypto/sha256"
	"path/filepath"
	"strings"

	"github.com/genesysflow/go-genesys/env"
	"github.com/genesysflow/go-genesys/foundation"
	"github.com/genesysflow/go-genesys/http"
	"github.com/genesysflow/go-genesys/http/middleware"
)

// GlobalMiddleware returns the global middleware stack.
func GlobalMiddleware(app *foundation.Application) []http.MiddlewareFunc {
	stack := []http.MiddlewareFunc{
		middleware.RequestID(),
		middleware.Logger(app.GetLogger()),
		middleware.Recover(app.GetLogger()),

		// The headers that cost nothing and close whole classes of
		// attack: no MIME sniffing, no framing, no referrer leaking to
		// other sites. HSTS is deliberately left off until the operator
		// knows every subdomain is served over TLS.
		middleware.Secure(),

		middleware.Maintenance(middleware.MaintenanceConfig{
			Path: filepath.Join(app.BasePath(), middleware.DefaultDownFilePath),
		}),
	}

	// CORS belongs on the token API, which is meant to be called from
	// elsewhere - not on the session-backed HTML site, where advertising
	// every page as cross-origin readable buys nothing and gives away
	// what a cookie protects.
	stack = append(stack, apiCORS())

	// Forms are protected; the token API is exempt because it
	// authenticates with a bearer token rather than a cookie, and
	// without a cookie there is no cross-site request to forge.
	if secret := csrfSecret(app); len(secret) > 0 {
		stack = append(stack, middleware.CSRF(middleware.CSRFConfig{
			Secret:         secret,
			CookieInsecure: !app.IsProduction(),
			Except:         []string{"/api/"},
		}))
	}

	return stack
}

// apiCORS answers cross-origin requests for the JSON API only. Every
// other path is left without the headers, so a browser will not let
// another site read it.
func apiCORS() http.MiddlewareFunc {
	cors := middleware.CORS()

	return func(ctx *http.Context, next func() error) error {
		if !strings.HasPrefix(ctx.Path(), "/api/") {
			return next()
		}
		return cors(ctx, next)
	}
}

// csrfSecret derives the CSRF signing key from the application key, so
// every instance signs with the same secret.
func csrfSecret(app *foundation.Application) []byte {
	key := app.GetConfig().GetString("app.key")
	if key == "" {
		key = env.Get("APP_KEY", "")
	}
	if key == "" {
		// Without a key there is nothing safe to sign with, and a
		// per-process secret would reject valid tokens after a restart.
		app.GetLogger().Warn("CSRF protection is off: set APP_KEY to enable it")
		return nil
	}

	sum := sha256.Sum256([]byte(key))
	return sum[:]
}

// Register registers all application routes.
func Register(r *http.Router) {
	// Load web routes
	Web(r)

	// Load API routes (SQLC-based v1)
	API(r)

	// Load ORM-based routes (v2: ORM, form requests, pagination, views)
	ORM(r)

	// The blog: authentication, HTML pages and the token-authenticated
	// JSON API. It needs the container, so a wiring mistake surfaces at
	// boot rather than on the first request.
	if err := Blog(r.App(), r); err != nil {
		panic("routes: the blog could not be registered: " + err.Error())
	}

	// The development panel, when the environment allows it.
	Devtools(r)
}
