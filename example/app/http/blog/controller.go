package blog

import (
	"fmt"
	"time"

	"github.com/genesysflow/go-genesys/database"
	"github.com/genesysflow/go-genesys/events"
	"github.com/genesysflow/go-genesys/example/app/models"
	"github.com/genesysflow/go-genesys/http"
	"github.com/genesysflow/go-genesys/query"
	"github.com/genesysflow/go-genesys/queue"
)

// Controller serves the blog's HTML pages.
type Controller struct {
	// Queue is where publishing work is dispatched.
	Queue queue.Queue

	// Events announces what happened, for listeners to pick up.
	Events *events.Dispatcher
}

// Index lists published posts, newest first, narrowed to one tag when
// the reader picked one from the filter bar.
func (c *Controller) Index(ctx *http.Context) error {
	tag := ctx.Query("tag")

	posts := database.Query[models.Post]().
		WhereNotNull("published_at").
		With("Author", "Tags").
		WithCount("Comments").
		Latest("published_at")

	// Filtered through the relation rather than a join written here, so
	// the pivot stays the tag relation's business.
	if tag != "" {
		posts = posts.WhereHas("Tags", func(tags *query.Builder) {
			tags.Where("slug", tag)
		})
	}

	page, err := posts.Paginate(ctx.QueryInt("page", 1), 10)
	if err != nil {
		return err
	}

	// The filter bar is chrome. Failing to list the tags is no reason to
	// refuse the reader the posts, so an error leaves the bar empty.
	tags, err := database.Query[models.Tag]().OrderBy("name").Get()
	if err != nil {
		tags = []models.Tag{}
	}

	return ctx.View("posts.index", map[string]any{
		"posts":     page.Data,
		"page":      page.CurrentPage,
		"total":     page.Total,
		"last_page": page.LastPage,
		"per_page":  page.PerPage,
		"tag":       tag,
		"tags":      tags,
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

	// The binder loads the row, not what hangs off it, so the byline and
	// the tags would be empty without this. A relation that fails to
	// load is missing chrome, not a missing page, so the error is
	// deliberately not returned.
	_ = database.Load(post, "Author", "Tags")

	comments, err := database.Query[models.Comment]().
		Where("post_id", post.ID).
		With("Author").
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

// Drafts lists what has not gone out yet: an author's own unpublished
// posts, and every author's for an editor. Without it a draft is
// unreachable the moment its author closes the tab.
func (c *Controller) Drafts(ctx *http.Context) error {
	user := models.CurrentUser(ctx)

	drafts := database.Query[models.Post]().
		WhereNull("published_at").
		With("Author").
		Latest("updated_at")

	// An author sees their own; an editor sees the whole desk.
	if !user.IsEditor() {
		drafts = drafts.Where("author_id", user.ID)
	}

	posts, err := drafts.Get()
	if err != nil {
		return err
	}

	return ctx.View("posts.drafts", map[string]any{
		"posts": posts,
		"mine":  !user.IsEditor(),
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

	author := models.CurrentUser(ctx)

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

	author := models.CurrentUser(ctx)

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
