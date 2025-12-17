package controllers

import (
	"strconv"

	"github.com/genesysflow/go-genesys/contracts"
	"github.com/genesysflow/go-genesys/database"
	"github.com/genesysflow/go-genesys/example/app/models"
	"github.com/genesysflow/go-genesys/example/app/services"
	"github.com/genesysflow/go-genesys/facades/db"
	"github.com/genesysflow/go-genesys/http"
	"github.com/genesysflow/go-genesys/validation"
)

// UserController handles user-related requests.
type UserController struct {
	*http.Controller
	userService *services.UserService
}

// NewUserController creates a new UserController.
func NewUserController(app contracts.Application, userService *services.UserService) *UserController {
	return &UserController{
		Controller:  http.NewController(app),
		userService: userService,
	}
}

// Index handles GET /users - List all users.
func (c *UserController) Index(ctx *http.Context) error {
	users, err := database.All[models.User]()
	if err != nil {
		return c.Error(ctx, 500, "Internal Server Error", err.Error())
	}

	return c.Success(ctx, users)
}

// Show handles GET /users/:id - Get a single user.
func (c *UserController) Show(ctx *http.Context) error {
	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		return c.BadRequest(ctx, "Invalid user ID")
	}

	user, err := database.Find[models.User](int64(id))
	if err != nil {
		return c.Error(ctx, 500, "Internal Server Error", err.Error())
	}
	if user == nil {
		return c.NotFound(ctx, "User not found")
	}

	return c.Success(ctx, user)
}

// CreateUserRequest represents the request body for creating a user.
type CreateUserRequest struct {
	Name      string `json:"name" validate:"required,min=2,max=100"`
	Email     string `json:"email" validate:"required,email"`
	Birthdate string `json:"birthdate" validate:"required"`
}

// Store handles POST /users - Create a new user.
func (c *UserController) Store(ctx *http.Context) error {
	var req CreateUserRequest
	if err := ctx.Bind(&req); err != nil {
		return c.BadRequest(ctx, "Invalid request body")
	}

	// Basic validation
	validator := validation.New()
	result := validator.Validate(&req)
	if result.Fails() {
		return c.ValidationError(ctx, result.Messages())
	}

	// Check for duplicate email
	exists, err := db.Table("users").Where("email", "=", req.Email).Exists()
	if err != nil {
		return c.Error(ctx, 500, "Internal Server Error", err.Error())
	}
	if exists {
		return c.ValidationError(ctx, map[string][]string{
			"email": {"Email already exists"},
		})
	}

	user := &models.User{
		Name:      req.Name,
		Email:     req.Email,
		Birthdate: req.Birthdate,
	}

	id, err := database.Create(user)
	if err != nil {
		return c.Error(ctx, 500, "Internal Server Error", err.Error())
	}

	user.ID = id

	return c.Created(ctx, user, "User created successfully")
}

// UpdateUserRequest represents the request body for updating a user.
type UpdateUserRequest struct {
	Name      string `json:"name" validate:"omitempty,min=2,max=100"`
	Email     string `json:"email" validate:"omitempty,email"`
	Birthdate string `json:"birthdate" validate:"omitempty"`
}

// Update handles PUT /users/:id - Update a user.
func (c *UserController) Update(ctx *http.Context) error {
	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		return c.BadRequest(ctx, "Invalid user ID")
	}

	var req UpdateUserRequest
	if err := ctx.Bind(&req); err != nil {
		return c.BadRequest(ctx, "Invalid request body")
	}

	// Fetch user
	user, err := database.Find[models.User](int64(id))
	if err != nil {
		return c.Error(ctx, 500, "Internal Server Error", err.Error())
	}
	if user == nil {
		return c.NotFound(ctx, "User not found")
	}

	if req.Name != "" {
		user.Name = req.Name
	}
	if req.Email != "" {
		// Check for duplicate email
		emailExists, err := db.Table("users").Where("email", "=", req.Email).Where("id", "!=", id).Exists()
		if err != nil {
			return c.Error(ctx, 500, "Internal Server Error", err.Error())
		}
		if emailExists {
			return c.ValidationError(ctx, map[string][]string{
				"email": {"Email already exists"},
			})
		}
		user.Email = req.Email
	}
	if req.Birthdate != "" {
		user.Birthdate = req.Birthdate
	}

	_, err = database.Update(int64(id), user)
	if err != nil {
		return c.Error(ctx, 500, "Internal Server Error", err.Error())
	}

	return c.Success(ctx, user, "User updated successfully")
}

// Destroy handles DELETE /users/:id - Delete a user.
func (c *UserController) Destroy(ctx *http.Context) error {
	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		return c.BadRequest(ctx, "Invalid user ID")
	}

	// Check if user exists
	exists, err := db.Table("users").Where("id", "=", id).Exists()
	if err != nil {
		return c.Error(ctx, 500, "Internal Server Error", err.Error())
	}
	if !exists {
		return c.NotFound(ctx, "User not found")
	}

	_, err = database.Delete[models.User](int64(id))
	if err != nil {
		return c.Error(ctx, 500, "Internal Server Error", err.Error())
	}

	return c.Success(ctx, nil, "User deleted successfully")
}
