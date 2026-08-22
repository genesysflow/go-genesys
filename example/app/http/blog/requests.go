// Package blog holds the example blog's HTTP layer.
package blog

import (
	"fmt"
	"strings"

	"github.com/genesysflow/go-genesys/example/app/models"
	"github.com/genesysflow/go-genesys/http"
	"github.com/genesysflow/go-genesys/support"
)

// StorePostRequest is the new-post form. It shows the whole form-request
// lifecycle: normalising input, authorizing, dynamic rules, custom
// messages and a cross-field check.
type StorePostRequest struct {
	Title  string `json:"title" form:"title" validate:"required,min=3,max=120"`
	Slug   string `json:"slug" form:"slug" validate:"required,max=140"`
	Body   string `json:"body" form:"body" validate:"required,min=20"`
	Notify string `json:"notify" form:"notify"`
}

// PrepareForValidation derives the slug from the title when the form
// leaves it blank, so the field is optional to a human but required to
// the rules.
func (r *StorePostRequest) PrepareForValidation(ctx *http.Context) error {
	r.Title = support.Str.Squish(r.Title)
	if r.Slug == "" {
		r.Slug = support.Str.Of(r.Title).Slug().Limit(140, "").Value()
	}
	return nil
}

// Authorize refuses a request from a caller who may not write at all -
// checked before the rules, so an unauthorized caller never learns which
// fields exist.
//
// "create" has no instance to act on, so an empty post names the policy
// to ask; PostPolicy.Create takes only the user.
func (r *StorePostRequest) Authorize(ctx *http.Context) bool {
	return ctx.Can("create", &models.Post{})
}

// Rules adds what the struct tags cannot express: the slug has to be
// free, which only the database knows.
func (r *StorePostRequest) Rules() map[string]string {
	return map[string]string{"slug": "unique=posts.slug"}
}

// Messages replaces the wording for this form only.
func (r *StorePostRequest) Messages() map[string]string {
	return map[string]string{
		"title.required": "Give the post a title.",
		"body.min":       "Write at least a couple of sentences.",
	}
}

// Attributes renames fields in messages.
func (r *StorePostRequest) Attributes() map[string]string {
	return map[string]string{"body": "post body"}
}

// AfterValidation runs once the rules pass, for the checks a per-field
// rule cannot make.
func (r *StorePostRequest) AfterValidation(ctx *http.Context) map[string][]string {
	if strings.EqualFold(r.Title, r.Body) {
		return map[string][]string{"body": {"The post body must say more than its title."}}
	}
	return nil
}

// UpdatePostRequest is the edit form. The slug must stay unique, ignoring
// the post being edited - otherwise every save would collide with itself.
type UpdatePostRequest struct {
	Current string `json:"-" form:"-"`
	Title   string `json:"title" form:"title" validate:"required,min=3,max=120"`
	Slug    string `json:"slug" form:"slug" validate:"required,max=140"`
	Body    string `json:"body" form:"body" validate:"required,min=20"`
}

// PrepareForValidation reads the post being edited from the route. The
// route carries the slug the post has now, which is what the uniqueness
// check has to ignore - the submitted slug may be a new one.
func (r *UpdatePostRequest) PrepareForValidation(ctx *http.Context) error {
	r.Current = ctx.Param("post")
	r.Title = support.Str.Squish(r.Title)
	return nil
}

// Rules ignores this post's own row when checking the slug, keyed on the
// slug column rather than the id, since that is how the post is
// addressed.
func (r *UpdatePostRequest) Rules() map[string]string {
	return map[string]string{"slug": fmt.Sprintf("unique=posts.slug.%s.slug", r.Current)}
}

// CommentRequest is the reply form.
type CommentRequest struct {
	Body string `json:"body" form:"body" validate:"required,min=2,max=1000"`
}
