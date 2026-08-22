package routes

import (
	"errors"

	genesysauth "github.com/genesysflow/go-genesys/auth"
	"github.com/genesysflow/go-genesys/container"
	"github.com/genesysflow/go-genesys/contracts"
	"github.com/genesysflow/go-genesys/database"
	"github.com/genesysflow/go-genesys/events"
	appauth "github.com/genesysflow/go-genesys/example/app/http/auth"
	"github.com/genesysflow/go-genesys/example/app/http/blog"
	"github.com/genesysflow/go-genesys/example/app/models"
	"github.com/genesysflow/go-genesys/http"
	"github.com/genesysflow/go-genesys/queue"
)

// Blog registers the blog: the authentication flows, the HTML pages and
// the token-authenticated JSON API.
func Blog(app contracts.Application, r *http.Router) error {
	guard, gate, tokens, err := authServices(app)
	if err != nil {
		return err
	}

	dispatcher, _ := container.Resolve[*events.Dispatcher](app)
	jobQueue, _ := container.Resolve[*queue.Manager](app)

	var pushTo queue.Queue
	if jobQueue != nil {
		pushTo, _ = jobQueue.Connection()
	}

	// Authentication: the scaffolded controller, wired to this
	// application's user model.
	authController := &appauth.Controller{
		Guard: guard,
		Users: genesysauth.NewORMUserProvider[models.User](),
		Home:  "/posts",
		Create: func(email, hashedPassword string) (genesysauth.Authenticatable, error) {
			user := &models.User{
				Name:      email,
				Email:     email,
				Password:  hashedPassword,
				Birthdate: "1970-01-01",
				Role:      "author",
			}
			return user, database.Create(user)
		},
	}
	authController.Routes(r)

	// Who is reading, and what they may do, on every request - not only
	// the guarded ones. A public page has to know both: to show an edit
	// link, and to let a draft's author past the policy that hides it.
	r.Use(genesysauth.ResolveUser(guard), genesysauth.GateMiddleware(gate))

	controller := &blog.Controller{Queue: pushTo, Events: dispatcher}
	api := &blog.API{Tokens: tokens}

	// Public reading.
	r.GET("/posts", controller.Index).Name("posts.index")
	r.GET("/posts/:post", controller.Show).Name("posts.show")

	// Writing needs a session; each route is guarded by the policy that
	// governs it, so the handler never runs for a caller who may not.
	authenticated := genesysauth.Middleware(guard, genesysauth.MiddlewareOptions{RedirectTo: "/login"})

	r.GET("/drafts/new", controller.Create, authenticated).Name("posts.create")
	r.POST("/posts", controller.Store, authenticated)
	r.POST("/posts/:post/comments", controller.Comment, authenticated).Name("posts.comment")

	// Posts are addressed by slug throughout, so the guard binds on the
	// same column the handlers do.
	r.GET("/posts/:post/edit", controller.Edit, authenticated).
		Middleware(genesysauth.CanBy[models.Post](gate, "update", "post", "slug")).
		Name("posts.edit")
	r.PUT("/posts/:post", controller.Update, authenticated).
		Middleware(genesysauth.CanBy[models.Post](gate, "update", "post", "slug"))
	r.DELETE("/posts/:post", controller.Destroy, authenticated).
		Middleware(genesysauth.CanBy[models.Post](gate, "delete", "post", "slug"))

	// Publishing is an editor's call.
	r.POST("/posts/:post/publish", controller.Publish, authenticated).
		Middleware(genesysauth.CanBy[models.Post](gate, "publish", "post", "slug")).
		Name("posts.publish")

	// Issuing an API token is done from the session-authenticated site.
	r.POST("/api-tokens", api.IssueToken, authenticated).Name("tokens.store")

	// The JSON API authenticates with those tokens instead.
	tokenGuard := genesysauth.NewPersonalAccessTokenGuard("api", tokens, genesysauth.NewORMUserProvider[models.User]())
	apiAuth := genesysauth.Middleware(tokenGuard)

	r.Group("/api/blog", func(api2 *http.Router) {
		api2.GET("/posts", api.Index).Name("api.posts.index")
		api2.GET("/posts/:post", api.Show).Name("api.posts.show")
		api2.GET("/me", api.Me, genesysauth.RequireAbility("profile:read")).Name("api.me")
		api2.POST("/posts", api.Store, genesysauth.RequireAbility("posts:write")).Name("api.posts.store")
	}, apiAuth, genesysauth.GateMiddleware(gate))

	return nil
}

// authServices resolves the pieces the blog authenticates and authorizes
// with.
func authServices(app contracts.Application) (genesysauth.StatefulGuard, *genesysauth.Gate, *genesysauth.TokenRepository, error) {
	manager, err := container.Resolve[*genesysauth.Manager](app)
	if err != nil {
		return nil, nil, nil, err
	}

	guard, err := manager.Guard("web")
	if err != nil {
		return nil, nil, nil, err
	}

	stateful, ok := guard.(genesysauth.StatefulGuard)
	if !ok {
		return nil, nil, nil, errNotStateful
	}

	gate, err := container.Resolve[*genesysauth.Gate](app)
	if err != nil {
		return nil, nil, nil, err
	}

	tokens, err := container.Resolve[*genesysauth.TokenRepository](app)
	if err != nil {
		return nil, nil, nil, err
	}

	return stateful, gate, tokens, nil
}

// errNotStateful is returned when the web guard cannot log users in.
var errNotStateful = errors.New("routes: the web guard cannot log users in")
