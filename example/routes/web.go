package routes

import (
	"github.com/genesysflow/go-genesys/database"
	"github.com/genesysflow/go-genesys/example/app/models"
	"github.com/genesysflow/go-genesys/facades/storage"
	"github.com/genesysflow/go-genesys/http"
)

// Web registers web routes.
func Web(r *http.Router) {
	app := r.App()

	// The landing page. It used to answer with a JSON greeting, which
	// left anyone who had just started the server with nowhere to go;
	// it now shows what the blog holds and links into it.
	r.GET("/", func(ctx *http.Context) error {
		// Nothing here is worth a 500. A landing page that cannot reach
		// the database should still render, empty, so the reader can at
		// least find the navigation.
		posts, err := database.Query[models.Post]().
			WhereNotNull("published_at").
			With("Author").
			Latest("published_at").
			Limit(3).
			Get()
		if err != nil {
			posts = []models.Post{}
		}

		env := ""
		if app != nil {
			env = app.GetConfig().GetString("app.env")
		}

		return ctx.View("home", map[string]any{
			"posts": posts,
			"counts": map[string]any{
				"posts":    countOf[models.Post](),
				"users":    countOf[models.User](),
				"comments": countOf[models.Comment](),
			},
			"env": env,
		})
	}).Name("home")

	// Health check
	r.GET("/health", func(ctx *http.Context) error {
		return ctx.JSONResponse(map[string]any{
			"status": "healthy",
		})
	}).Name("health")
	// Filesystem test
	r.GET("/filesystem/test", func(ctx *http.Context) error {
		// Assuming context comes from fiber context context
		c := ctx.FiberCtx().Context() // userContext
		if err := storage.Put(c, "test.txt", "Hello Filesystem!"); err != nil {
			return err
		}
		content, err := storage.Get(c, "test.txt")
		if err != nil {
			return err
		}
		return ctx.JSONResponse(map[string]any{
			"content": content,
			"exists":  storage.Exists(c, "test.txt"),
			"path":    storage.Url("test.txt"),
		})
	})
}

// countOf totals a table for the landing page's summary, reporting zero
// when the count cannot be taken: a missing number is a cosmetic loss,
// and not worth refusing to render the page over.
func countOf[T any]() int64 {
	total, err := database.Query[T]().Count()
	if err != nil {
		return 0
	}
	return total
}
