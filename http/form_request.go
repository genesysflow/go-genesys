package http

import (
	"github.com/genesysflow/go-genesys/container"
	"github.com/genesysflow/go-genesys/errors"
	"github.com/genesysflow/go-genesys/validation"
)

// ValidateRequest binds the request payload into T and validates it using
// the struct's `validate` tags, Laravel form-request style:
//
//	type StoreUserRequest struct {
//	    Name  string `json:"name" validate:"required,min=3"`
//	    Email string `json:"email" validate:"required,email"`
//	}
//
//	func StoreUser(ctx *http.Context) error {
//	    req, err := http.ValidateRequest[StoreUserRequest](ctx)
//	    if err != nil {
//	        return err // rendered as 422 {"message": ..., "errors": {...}}
//	    }
//	    ...
//	}
//
// JSON bodies, form bodies, and (for GET/HEAD/DELETE) query strings are
// bound based on the request. Validation failures return an
// *errors.ValidationError which the framework error handler renders as a
// 422 response with per-field messages.
func ValidateRequest[T any](ctx *Context) (*T, error) {
	req := new(T)

	if err := bindRequest(ctx, req); err != nil {
		return nil, errors.BadRequest("The request payload could not be parsed.", err)
	}

	validator, err := container.Resolve[*validation.Validator](ctx.App())
	if err != nil {
		// No validator registered: fall back to a fresh instance so form
		// requests still work in minimal apps.
		validator = validation.New()
	}

	result := validator.Validate(req)
	if result.Fails() {
		return nil, errors.NewValidationError(result.Errors().All())
	}

	return req, nil
}

// bindRequest fills v from the query string (bodyless methods) or the
// request body (JSON, form, multipart) based on the request.
func bindRequest(ctx *Context, v any) error {
	switch ctx.Method() {
	case "GET", "HEAD", "DELETE":
		return ctx.FiberCtx().QueryParser(v)
	default:
		if len(ctx.FiberCtx().Body()) == 0 {
			return ctx.FiberCtx().QueryParser(v)
		}
		return ctx.FiberCtx().BodyParser(v)
	}
}
