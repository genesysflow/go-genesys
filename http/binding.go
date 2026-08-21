package http

import (
	goerrors "errors"
	"strconv"

	"github.com/genesysflow/go-genesys/database"
	"github.com/genesysflow/go-genesys/errors"
)

// BindModel resolves a route parameter into a model, Laravel
// route-model-binding style. A missing record becomes a 404 error:
//
//	router.GET("/users/:user", func(ctx *http.Context) error {
//	    user, err := http.BindModel[models.User](ctx, "user")
//	    if err != nil {
//	        return err
//	    }
//	    return ctx.Resource(user)
//	})
func BindModel[T any](ctx *Context, param string) (*T, error) {
	raw := ctx.Param(param)
	if raw == "" {
		return nil, errors.NotFound()
	}

	var id any = raw
	if n, err := strconv.ParseInt(raw, 10, 64); err == nil {
		id = n
	}

	model, err := database.Find[T](id)
	if err != nil {
		if goerrors.Is(err, database.ErrNotFound) {
			return nil, errors.NotFound()
		}
		return nil, err
	}
	return model, nil
}

// BindModelBy resolves a route parameter against a custom column
// (e.g. a slug) instead of the primary key.
func BindModelBy[T any](ctx *Context, param, column string) (*T, error) {
	raw := ctx.Param(param)
	if raw == "" {
		return nil, errors.NotFound()
	}
	model, err := database.FirstWhere[T](column, raw)
	if err != nil {
		if goerrors.Is(err, database.ErrNotFound) {
			return nil, errors.NotFound()
		}
		return nil, err
	}
	return model, nil
}
