package blog

import (
	"fmt"
	"time"

	genesysauth "github.com/genesysflow/go-genesys/auth"
	"github.com/genesysflow/go-genesys/database"
	"github.com/genesysflow/go-genesys/events"
	"github.com/genesysflow/go-genesys/example/app/models"
	"github.com/genesysflow/go-genesys/http"
	"github.com/genesysflow/go-genesys/queue"
)

// Controller serves the blog's HTML pages.
type Controller struct {
	// Queue is where publishing work is dispatched.
	Queue queue.Queue

	// Events announces what happened, for listeners to pick up.
	Events *events.Dispatcher
}

// Index lists published posts, newest first.
func (c *Controller) Index(ctx *http.Context) error {
	page, err := database.Query[models.Post]().
		WhereNotNull("published_at").
		With("Author", "Tags").
		WithCount("Comments").
		Latest("published_at").
		Paginate(ctx.QueryInt("page", 1), 10)
	if err != nil {
		return err
	}

	return ctx.View("posts.index", map[string]any{
		"posts": page.Data,
		"page":  page.CurrentPage,
		"total": page.Total,
	})
}

// Show renders one post with its comments.
func (c *Controller) Show(ctx *http.Context) error {
	post, err := http.BindModelBy[models.Post](ctx, "post", "slug")
	if err != nil {
		return err
	}

	// A draft is nobody's business but its author's and the editors'.
	if err := ctx.Authorize("view", post); err != nil {
		return err
	}

	comments, err := database.Query[models.Comment]().
		Where("post_id", post.ID).
		OrderBy("id").
		Get()
	if err != nil {
		return err
	}

	return ctx.View("posts.show", map[string]any{
		"post":     post,
		"comments": comments,
	})
}

// Create renders the new-post form.
func (c *Controller) Create(ctx *http.Context) error {
	return ctx.View("posts.create")
}

// Store validates and writes a new post. A validation failure never
// reaches here: the framework sends the browser back to the form with
// its input and messages.
func (c *Controller) Store(ctx *http.Context) error {
	req, err := http.ValidateRequest[StorePostRequest](ctx)
	if err != nil {
		return err
	}

	author := genesysauth.UserFrom(ctx).(*models.User)

	post := &models.Post{
		AuthorID: author.ID,
		Title:    req.Title,
		Slug:     req.Slug,
		Body:     req.Body,
	}
	if err := database.Create(post); err != nil {
		return err
	}

	return ctx.RedirectToRoute("posts.show", map[string]any{"post": post.Slug}).
		With("status", "Draft saved.").
		Send()
}

// Edit renders the edit form for a post the caller may change.
func (c *Controller) Edit(ctx *http.Context) error {
	post, err := http.BindModelBy[models.Post](ctx, "post", "slug")
	if err != nil {
		return err
	}

	return ctx.View("posts.edit", map[string]any{"post": post})
}

// Update saves an edit.
func (c *Controller) Update(ctx *http.Context) error {
	post, err := http.BindModelBy[models.Post](ctx, "post", "slug")
	if err != nil {
		return err
	}

	req, err := http.ValidateRequest[UpdatePostRequest](ctx)
	if err != nil {
		return err
	}

	post.Title = req.Title
	post.Slug = req.Slug
	post.Body = req.Body

	// Only the changed columns are written, so two editors touching
	// different fields do not overwrite each other.
	if err := database.Update(post); err != nil {
		return err
	}

	return ctx.RedirectToRoute("posts.show", map[string]any{"post": post.Slug}).
		With("status", "Post updated.").
		Send()
}

// Destroy soft-deletes a post.
func (c *Controller) Destroy(ctx *http.Context) error {
	post, err := http.BindModelBy[models.Post](ctx, "post", "slug")
	if err != nil {
		return err
	}

	if err := database.DeleteModel(post); err != nil {
		return err
	}

	return ctx.RedirectToRoute("posts.index").With("status", "Post deleted.").Send()
}

// Publish queues the publishing work and announces it. The response
// returns immediately; the worker does the rest.
func (c *Controller) Publish(ctx *http.Context) error {
	post, err := http.BindModelBy[models.Post](ctx, "post", "slug")
	if err != nil {
		return err
	}

	published := time.Now()
	post.PublishedAt = &published
	if err := database.Update(post); err != nil {
		return err
	}

	if c.Events != nil {
		if err := c.Events.Dispatch(PostPublished{PostID: post.ID, Title: post.Title}); err != nil {
			return err
		}
	}

	return ctx.RedirectToRoute("posts.show", map[string]any{"post": post.Slug}).
		With("status", fmt.Sprintf("%q is live.", post.Title)).
		Send()
}

// Comment stores a reply to a post.
func (c *Controller) Comment(ctx *http.Context) error {
	post, err := http.BindModelBy[models.Post](ctx, "post", "slug")
	if err != nil {
		return err
	}

	req, err := http.ValidateRequest[CommentRequest](ctx)
	if err != nil {
		return err
	}

	author := genesysauth.UserFrom(ctx).(*models.User)

	if err := database.Create(&models.Comment{
		PostID:   post.ID,
		AuthorID: author.ID,
		Body:     req.Body,
	}); err != nil {
		return err
	}

	return ctx.Back("/").With("status", "Comment posted.").Send()
}

// PostPublished is dispatched when a post goes live.
type PostPublished struct {
	PostID int64
	Title  string
}

// Name identifies the event to listeners.
func (e PostPublished) Name() string { return "post.published" }

// PostIdentifier lets a listener read the post's id without importing
// this package.
func (e PostPublished) PostIdentifier() int64 { return e.PostID }
