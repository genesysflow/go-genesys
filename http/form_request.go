package http

import (
	"github.com/genesysflow/go-genesys/container"
	"github.com/genesysflow/go-genesys/errors"
	"github.com/genesysflow/go-genesys/validation"
)

// A form request may implement any of the following interfaces on its
// pointer receiver to take part in the request lifecycle, Laravel's
// FormRequest hooks. All of them are optional: a plain struct with
// `validate` tags keeps working exactly as before.

// PreparesForValidation normalises the payload before the rules run -
// trimming, defaulting, deriving a slug from a title.
type PreparesForValidation interface {
	PrepareForValidation(ctx *Context) error
}

// AuthorizesRequest decides whether the caller may make this request at
// all. Returning false is a 403, checked before validation so an
// unauthorized request never learns which fields exist.
type AuthorizesRequest interface {
	Authorize(ctx *Context) bool
}

// ProvidesRules supplies rules that struct tags cannot express, because
// they depend on the payload or on application state. They are applied to
// the raw request input on top of the struct's own tag rules.
type ProvidesRules interface {
	Rules() map[string]string
}

// ProvidesMessages overrides the message for a field/rule pair, keyed
// "<field>.<rule>" (e.g. "email.required").
type ProvidesMessages interface {
	Messages() map[string]string
}

// ProvidesAttributes renames fields in messages, keyed by field name
// (e.g. "email_addr" -> "email address").
type ProvidesAttributes interface {
	Attributes() map[string]string
}

// ValidatesAfter runs once the rules have passed and reports any further
// failures - the cross-field checks a per-field rule cannot express. It
// is skipped when the rules already failed, so it never reports on values
// that were rejected before it.
type ValidatesAfter interface {
	AfterValidation(ctx *Context) map[string][]string
}

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
//	        return err // rendered as 422, or a redirect back for browsers
//	    }
//	    ...
//	}
//
// JSON bodies, form bodies, and (for GET/HEAD/DELETE) query strings are
// bound based on the request. T may implement PreparesForValidation,
// AuthorizesRequest, ProvidesRules, ProvidesMessages, ProvidesAttributes,
// and ValidatesAfter to take part in the lifecycle.
//
// Validation failures return an *errors.ValidationError, which the
// framework renders as a 422 with per-field messages for API clients and
// as a redirect back to the form (errors and old input flashed) for
// browsers.
func ValidateRequest[T any](ctx *Context) (*T, error) {
	req := new(T)

	if err := bindRequest(ctx, req); err != nil {
		return nil, errors.BadRequest("The request payload could not be parsed.", err)
	}

	if preparer, ok := any(req).(PreparesForValidation); ok {
		if err := preparer.PrepareForValidation(ctx); err != nil {
			return nil, err
		}
	}

	if authorizer, ok := any(req).(AuthorizesRequest); ok {
		if !authorizer.Authorize(ctx) {
			return nil, errors.Forbidden("This action is unauthorized.")
		}
	}

	validator := requestValidator(ctx, req)

	failures := validator.Validate(req).Messages()

	// Rules() covers what tags cannot: rules that depend on the payload
	// or on application state. They run against the raw input.
	if ruled, ok := any(req).(ProvidesRules); ok {
		if rules := ruled.Rules(); len(rules) > 0 {
			mergeFailures(&failures, validator.ValidateMap(ctx.All(), rules).Messages())
		}
	}

	if len(failures) > 0 {
		return nil, errors.NewValidationError(failures)
	}

	if after, ok := any(req).(ValidatesAfter); ok {
		if extra := after.AfterValidation(ctx); len(extra) > 0 {
			return nil, errors.NewValidationError(extra)
		}
	}

	return req, nil
}

// requestValidator resolves the application validator and layers this
// request's own messages and attribute names over it.
func requestValidator(ctx *Context, req any) *validation.Validator {
	validator, err := container.Resolve[*validation.Validator](ctx.App())
	if err != nil {
		// No validator registered: fall back to a fresh instance so form
		// requests still work in minimal apps.
		validator = validation.New()
	}

	var messages, attributes map[string]string
	if provider, ok := req.(ProvidesMessages); ok {
		messages = provider.Messages()
	}
	if provider, ok := req.(ProvidesAttributes); ok {
		attributes = provider.Attributes()
	}

	return validator.WithOverrides(messages, attributes)
}

// mergeFailures folds extra messages into failures, allocating only when
// there is something to fold in.
func mergeFailures(failures *map[string][]string, extra map[string][]string) {
	if len(extra) == 0 {
		return
	}
	if *failures == nil {
		*failures = make(map[string][]string, len(extra))
	}
	for field, messages := range extra {
		(*failures)[field] = append((*failures)[field], messages...)
	}
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
