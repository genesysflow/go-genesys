package http

import "github.com/genesysflow/go-genesys/contracts"

// Controller is the base controller with common functionality.
type Controller struct {
	app contracts.Application
}

// NewController creates a new base controller.
func NewController(app contracts.Application) *Controller {
	return &Controller{app: app}
}

// App returns the application instance.
func (c *Controller) App() contracts.Application {
	return c.app
}

// SetApp sets the application instance.
func (c *Controller) SetApp(app contracts.Application) {
	c.app = app
}

// JSON helper for returning JSON responses.
func (c *Controller) JSON(ctx *Context, data any) error {
	return ctx.JSONResponse(data)
}

// Success helper for returning success responses.
func (c *Controller) Success(ctx *Context, data any, message ...string) error {
	response := map[string]any{
		"success": true,
		"data":    data,
	}
	if len(message) > 0 {
		response["message"] = message[0]
	}
	return ctx.JSONResponse(response)
}

// Error helper for returning error responses.
func (c *Controller) Error(ctx *Context, code int, message string, errors ...any) error {
	response := map[string]any{
		"success": false,
		"error":   message,
	}
	if len(errors) > 0 {
		response["errors"] = errors[0]
	}
	return ctx.Status(code).JSONResponse(response)
}

// Created helper for returning 201 responses.
func (c *Controller) Created(ctx *Context, data any, message ...string) error {
	response := map[string]any{
		"success": true,
		"data":    data,
	}
	if len(message) > 0 {
		response["message"] = message[0]
	}
	return ctx.Status(201).JSONResponse(response)
}

// NoContent helper for returning 204 responses.
func (c *Controller) NoContent(ctx *Context) error {
	return ctx.NoContent()
}

// NotFound helper for returning 404 responses.
func (c *Controller) NotFound(ctx *Context, message ...string) error {
	msg := "Resource not found"
	if len(message) > 0 {
		msg = message[0]
	}
	return c.Error(ctx, 404, msg)
}

// BadRequest helper for returning 400 responses.
func (c *Controller) BadRequest(ctx *Context, message string, errors ...any) error {
	return c.Error(ctx, 400, message, errors...)
}

// Unauthorized helper for returning 401 responses.
func (c *Controller) Unauthorized(ctx *Context, message ...string) error {
	msg := "Unauthorized"
	if len(message) > 0 {
		msg = message[0]
	}
	return c.Error(ctx, 401, msg)
}

// Forbidden helper for returning 403 responses.
func (c *Controller) Forbidden(ctx *Context, message ...string) error {
	msg := "Forbidden"
	if len(message) > 0 {
		msg = message[0]
	}
	return c.Error(ctx, 403, msg)
}

// ValidationError helper for returning validation error responses.
func (c *Controller) ValidationError(ctx *Context, errors any) error {
	return c.Error(ctx, 422, "Validation failed", errors)
}

// Paginate helper for returning paginated responses.
func (c *Controller) Paginate(ctx *Context, data any, page, perPage, total int) error {
	totalPages := (total + perPage - 1) / perPage
	return ctx.JSONResponse(map[string]any{
		"success": true,
		"data":    data,
		"meta": map[string]any{
			"page":        page,
			"per_page":    perPage,
			"total":       total,
			"total_pages": totalPages,
			"has_more":    page < totalPages,
		},
	})
}

