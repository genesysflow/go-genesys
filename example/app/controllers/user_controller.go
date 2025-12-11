package controllers

import (
	"strconv"
	"sync"

	"github.com/genesysflow/go-genesys/contracts"
	"github.com/genesysflow/go-genesys/http"
)

// User represents a user model.
type User struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
	Age   int    `json:"age"`
}

// UserController handles user-related requests.
type UserController struct {
	*http.Controller
	users  map[int]*User
	nextID int
	mu     sync.RWMutex
}

// NewUserController creates a new UserController.
func NewUserController(app contracts.Application) *UserController {
	return &UserController{
		Controller: http.NewController(app),
		users: map[int]*User{
			1: {ID: 1, Name: "John Doe", Email: "john@example.com", Age: 30},
			2: {ID: 2, Name: "Jane Smith", Email: "jane@example.com", Age: 25},
			3: {ID: 3, Name: "Bob Wilson", Email: "bob@example.com", Age: 35},
		},
		nextID: 4,
	}
}

// Index handles GET /users - List all users.
func (c *UserController) Index(ctx *http.Context) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	users := make([]*User, 0, len(c.users))
	for _, user := range c.users {
		users = append(users, user)
	}

	return c.Success(ctx, users)
}

// Show handles GET /users/:id - Get a single user.
func (c *UserController) Show(ctx *http.Context) error {
	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		return c.BadRequest(ctx, "Invalid user ID")
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	user, ok := c.users[id]
	if !ok {
		return c.NotFound(ctx, "User not found")
	}

	return c.Success(ctx, user)
}

// CreateUserRequest represents the request body for creating a user.
type CreateUserRequest struct {
	Name  string `json:"name" validate:"required,min=2,max=100"`
	Email string `json:"email" validate:"required,email"`
	Age   int    `json:"age" validate:"required,gte=0,lte=150"`
}

// Store handles POST /users - Create a new user.
func (c *UserController) Store(ctx *http.Context) error {
	var req CreateUserRequest
	if err := ctx.Bind(&req); err != nil {
		return c.BadRequest(ctx, "Invalid request body")
	}

	// Basic validation
	if req.Name == "" {
		return c.ValidationError(ctx, map[string][]string{
			"name": {"Name is required"},
		})
	}
	if req.Email == "" {
		return c.ValidationError(ctx, map[string][]string{
			"email": {"Email is required"},
		})
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Check for duplicate email
	for _, user := range c.users {
		if user.Email == req.Email {
			return c.ValidationError(ctx, map[string][]string{
				"email": {"Email already exists"},
			})
		}
	}

	user := &User{
		ID:    c.nextID,
		Name:  req.Name,
		Email: req.Email,
		Age:   req.Age,
	}
	c.users[user.ID] = user
	c.nextID++

	return c.Created(ctx, user, "User created successfully")
}

// UpdateUserRequest represents the request body for updating a user.
type UpdateUserRequest struct {
	Name  string `json:"name" validate:"omitempty,min=2,max=100"`
	Email string `json:"email" validate:"omitempty,email"`
	Age   int    `json:"age" validate:"omitempty,gte=0,lte=150"`
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

	c.mu.Lock()
	defer c.mu.Unlock()

	user, ok := c.users[id]
	if !ok {
		return c.NotFound(ctx, "User not found")
	}

	// Update fields if provided
	if req.Name != "" {
		user.Name = req.Name
	}
	if req.Email != "" {
		// Check for duplicate email
		for _, u := range c.users {
			if u.ID != id && u.Email == req.Email {
				return c.ValidationError(ctx, map[string][]string{
					"email": {"Email already exists"},
				})
			}
		}
		user.Email = req.Email
	}
	if req.Age > 0 {
		user.Age = req.Age
	}

	return c.Success(ctx, user, "User updated successfully")
}

// Destroy handles DELETE /users/:id - Delete a user.
func (c *UserController) Destroy(ctx *http.Context) error {
	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		return c.BadRequest(ctx, "Invalid user ID")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.users[id]; !ok {
		return c.NotFound(ctx, "User not found")
	}

	delete(c.users, id)

	return c.Success(ctx, nil, "User deleted successfully")
}

