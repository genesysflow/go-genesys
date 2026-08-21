package routes

import (
	"time"

	"github.com/genesysflow/go-genesys/database"
	"github.com/genesysflow/go-genesys/example/app/models"
	"github.com/genesysflow/go-genesys/facades/cache"
	"github.com/genesysflow/go-genesys/http"
)

// ORM registers routes demonstrating the framework ORM, form requests,
// pagination, route-model binding, caching, and views.
func ORM(r *http.Router) {
	r.Group("/api/v2", func(api *http.Router) {
		api.GET("/users", listUsersORM).Name("api.v2.users.index")
		api.POST("/users", createUserORM).Name("api.v2.users.store")
		api.GET("/users/:user", showUserORM).Name("api.v2.users.show")
		api.DELETE("/users/:user", deleteUserORM).Name("api.v2.users.destroy")
		api.GET("/stats", statsORM).Name("api.v2.stats")
	})

	// Server-rendered view (requires the ViewServiceProvider).
	r.GET("/welcome", func(ctx *http.Context) error {
		return ctx.View("welcome", map[string]any{
			"title": "Go-Genesys",
			"now":   time.Now().Format(time.RFC1123),
		})
	}).Name("welcome")
}

// listUsersORM demonstrates typed queries with Laravel-style pagination:
// GET /api/v2/users?page=2&per_page=10&search=ali
func listUsersORM(ctx *http.Context) error {
	query := database.Query[models.User]().OrderBy("id")
	if search := ctx.Query("search"); search != "" {
		query.Where("name", "like", "%"+search+"%")
	}

	page, err := query.Paginate(ctx.QueryInt("page", 1), ctx.QueryInt("per_page", 15))
	if err != nil {
		return err
	}
	return ctx.Paginated(page)
}

// storeUserRequest is a validated form request.
type storeUserRequest struct {
	Name      string `json:"name" validate:"required,min=2,max=100"`
	Email     string `json:"email" validate:"required,email"`
	Birthdate string `json:"birthdate" validate:"required"`
}

// createUserORM demonstrates ValidateRequest + ORM Create:
// invalid payloads automatically become 422 {"message", "errors"} responses.
func createUserORM(ctx *http.Context) error {
	req, err := http.ValidateRequest[storeUserRequest](ctx)
	if err != nil {
		return err
	}

	user := &models.User{Name: req.Name, Email: req.Email, Birthdate: req.Birthdate}
	if err := database.Create(user); err != nil {
		return err
	}
	return ctx.Status(201).JSONResponse(map[string]any{"data": user})
}

// showUserORM demonstrates route-model binding: a missing id is a 404.
func showUserORM(ctx *http.Context) error {
	user, err := http.BindModel[models.User](ctx, "user")
	if err != nil {
		return err
	}
	return ctx.Resource(user)
}

// deleteUserORM demonstrates BindModel + ORM delete.
func deleteUserORM(ctx *http.Context) error {
	user, err := http.BindModel[models.User](ctx, "user")
	if err != nil {
		return err
	}
	if err := database.DeleteModel(user); err != nil {
		return err
	}
	return ctx.NoContent()
}

// statsORM demonstrates cache.Remember over an aggregate query.
func statsORM(ctx *http.Context) error {
	total, err := cache.Remember("stats.users.total", time.Minute, func() (any, error) {
		count, err := database.Query[models.User]().Count()
		return count, err
	})
	if err != nil {
		return err
	}
	return ctx.JSONResponse(map[string]any{
		"users":     total,
		"cached_at": time.Now().Format(time.RFC3339),
	})
}
