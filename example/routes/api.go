package routes

import (
	"github.com/genesysflow/go-genesys/example/app/controllers"
	"github.com/genesysflow/go-genesys/http"
	"github.com/genesysflow/go-genesys/validation"
)

// API registers API routes.
func API(r *http.Router) {
	// Create controllers
	userController := controllers.NewUserController(nil)

	// API v1 routes
	r.Group("/api/v1", func(api *http.Router) {
		// Users resource
		api.GET("/users", userController.Index).Name("api.users.index")
		api.POST("/users", userController.Store).Name("api.users.store")
		api.GET("/users/:id", userController.Show).Name("api.users.show")
		api.PUT("/users/:id", userController.Update).Name("api.users.update")
		api.DELETE("/users/:id", userController.Destroy).Name("api.users.destroy")

		// Example validation route
		api.POST("/validate", func(ctx *http.Context) error {
			type CreateUserRequest struct {
				Name  string `json:"name" validate:"required,min=2,max=100"`
				Email string `json:"email" validate:"required,email"`
				Age   int    `json:"age" validate:"required,gte=18,lte=120"`
			}

			var req CreateUserRequest
			if err := ctx.Bind(&req); err != nil {
				return ctx.BadRequest("Invalid JSON body")
			}

			validator := validation.New()
			result := validator.Validate(&req)
			if result.Fails() {
				return ctx.Status(422).JSONResponse(map[string]any{
					"success": false,
					"message": "Validation failed",
					"errors":  result.Messages(),
				})
			}

			return ctx.JSONResponse(map[string]any{
				"success": true,
				"message": "Validation passed",
				"data":    req,
			})
		}).Name("api.validate")
	})
}
