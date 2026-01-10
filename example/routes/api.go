package routes

import (
	"context"
	"strconv"

	exampledb "github.com/genesysflow/go-genesys/example/database/db"
	"github.com/genesysflow/go-genesys/facades/db"
	"github.com/genesysflow/go-genesys/http"
	"github.com/genesysflow/go-genesys/validation"
)

// API registers API routes.
func API(r *http.Router) {
	// API v1 routes
	r.Group("/api/v1", func(api *http.Router) {
		// Users CRUD using SQLC-generated queries
		api.GET("/users", listUsers).Name("api.users.index")
		api.POST("/users", createUser).Name("api.users.store")
		api.GET("/users/:id", getUser).Name("api.users.show")
		api.DELETE("/users/:id", deleteUser).Name("api.users.destroy")

		// Example validation route
		api.POST("/validate", validateExample).Name("api.validate")
	})
}

// listUsers handles GET /api/v1/users
func listUsers(ctx *http.Context) error {
	queries := exampledb.New(db.DB())
	users, err := queries.ListUsers(context.Background())
	if err != nil {
		return ctx.Status(500).JSONResponse(map[string]any{
			"success": false,
			"message": "Failed to fetch users",
			"error":   err.Error(),
		})
	}
	return ctx.JSONResponse(map[string]any{
		"success": true,
		"data":    users,
	})
}

// createUser handles POST /api/v1/users
func createUser(ctx *http.Context) error {
	type CreateUserRequest struct {
		Name      string `json:"name" validate:"required,min=2,max=100"`
		Email     string `json:"email" validate:"required,email"`
		Birthdate string `json:"birthdate"`
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

	queries := exampledb.New(db.DB())
	user, err := queries.CreateUser(context.Background(), exampledb.CreateUserParams{
		Name:      req.Name,
		Email:     req.Email,
		Birthdate: req.Birthdate,
	})
	if err != nil {
		return ctx.Status(500).JSONResponse(map[string]any{
			"success": false,
			"message": "Failed to create user",
			"error":   err.Error(),
		})
	}

	return ctx.Status(201).JSONResponse(map[string]any{
		"success": true,
		"message": "User created successfully",
		"data":    user,
	})
}

// getUser handles GET /api/v1/users/:id
func getUser(ctx *http.Context) error {
	idStr := ctx.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return ctx.BadRequest("Invalid user ID")
	}

	queries := exampledb.New(db.DB())
	user, err := queries.GetUser(context.Background(), id)
	if err != nil {
		return ctx.Status(404).JSONResponse(map[string]any{
			"success": false,
			"message": "User not found",
		})
	}

	return ctx.JSONResponse(map[string]any{
		"success": true,
		"data":    user,
	})
}

// deleteUser handles DELETE /api/v1/users/:id
func deleteUser(ctx *http.Context) error {
	idStr := ctx.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return ctx.BadRequest("Invalid user ID")
	}

	queries := exampledb.New(db.DB())
	if err := queries.DeleteUser(context.Background(), id); err != nil {
		return ctx.Status(500).JSONResponse(map[string]any{
			"success": false,
			"message": "Failed to delete user",
			"error":   err.Error(),
		})
	}

	return ctx.JSONResponse(map[string]any{
		"success": true,
		"message": "User deleted successfully",
	})
}

// validateExample handles POST /api/v1/validate
func validateExample(ctx *http.Context) error {
	type CreateUserRequest struct {
		Name      string `json:"name" validate:"required,min=2,max=100"`
		Email     string `json:"email" validate:"required,email"`
		Birthdate string `json:"birthdate" validate:"required"`
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
}
