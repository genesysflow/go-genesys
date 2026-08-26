package routes

import (
	"crypto/sha256"
	"path/filepath"
	"strings"

	"github.com/genesysflow/go-genesys/env"
	"github.com/genesysflow/go-genesys/foundation"
	"github.com/genesysflow/go-genesys/http"
	"github.com/genesysflow/go-genesys/http/middleware"
	"github.com/gofiber/fiber/v2"
)

// PreRouting returns the handlers that run before the router matches a
// request, and so before every middleware in GlobalMiddleware.
//
// Only what has to decide *what the request is* belongs here. Everything
// else is ordinary middleware and belongs in the stack below, where it is
// easier to reason about.
func PreRouting() []fiber.Handler {
	return []fiber.Handler{
		// The blog's edit, delete and revoke buttons are HTML forms, and
		// an HTML form can only be submitted as GET or POST. This turns
		// the _method field that method_field renders into the verb the
		// router dispatches on, so those buttons reach the PUT and DELETE
		// routes a script reaches directly.
		//
		// It cannot be moved into GlobalMiddleware: middleware there runs
		// inside the handler the router already picked, and a POST that
		// meant DELETE has been answered 405 long before it gets a turn.
		//
		// Running before the router also settles the verb before anything
		// that branches on it - the CSRF check keys off the method, and
		// the logger records it. Both should be looking at the request
		// being served rather than at the shape a browser had to send it
		// in.
		middleware.MethodOverride(),
	}
}

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
	//
	// This sees the verb PreRouting settled, not the one on the wire: a
	// form's POST that declared DELETE is checked as the DELETE it is.
	// Do not move MethodOverride into this stack to "tidy" the ordering -
	// it would stop working entirely; see PreRouting above.
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
	// Stylesheets and scripts, so {{asset "css/app.css"}} resolves to a
	// file rather than a 404. The root is taken from the application's
	// base path rather than the working directory, because the test
	// suite runs from example/tests and would otherwise look for
	// public/ inside it.
	if app := r.App(); app != nil {
		r.Static("/assets", filepath.Join(app.BasePath(), "public"))
	}

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
