package blog

import (
	genesysauth "github.com/genesysflow/go-genesys/auth"
	"github.com/genesysflow/go-genesys/database"
	"github.com/genesysflow/go-genesys/example/app/models"
	"github.com/genesysflow/go-genesys/http"
)

// API serves the blog as JSON, authenticated with personal access
// tokens rather than a session.
type API struct {
	// Tokens issues the tokens the API authenticates with.
	Tokens *genesysauth.TokenRepository
}

// Index returns published posts, paginated.
func (a *API) Index(ctx *http.Context) error {
	page, err := database.Query[models.Post]().
		WhereNotNull("published_at").
		With("Author", "Tags").
		WithCount("Comments").
		Latest("published_at").
		Paginate(ctx.QueryInt("page", 1), 15)
	if err != nil {
		return err
	}

	// Rendered through ToMap, so Hidden and Appends are honoured: the
	// soft-delete column stays out and the computed fields come along.
	return ctx.ResourceWith(database.ToMapSlice(page.Data), map[string]any{
		"current_page": page.CurrentPage,
		"per_page":     page.PerPage,
		"total":        page.Total,
		"last_page":    page.LastPage,
	})
}

// Show returns one post by slug.
func (a *API) Show(ctx *http.Context) error {
	post, err := http.BindModelBy[models.Post](ctx, "post", "slug")
	if err != nil {
		return err
	}
	if err := ctx.Authorize("view", post); err != nil {
		return err
	}

	return ctx.Resource(database.ToMap(post))
}

// Store writes a post for the token's owner.
func (a *API) Store(ctx *http.Context) error {
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

	return ctx.Created(database.ToMap(post))
}

// Me describes the caller and the token they used.
func (a *API) Me(ctx *http.Context) error {
	user, ok := http.UserAs[models.User](ctx)
	if !ok {
		return ctx.Unauthorized()
	}

	payload := map[string]any{"user": database.ToMap(user)}

	if token := genesysauth.TokenFrom(ctx); token != nil {
		payload["token"] = map[string]any{
			"name":      token.Name,
			"abilities": token.Abilities,
		}
	}

	return ctx.Resource(payload)
}

// IssueToken mints a token for the signed-in user - the bridge from the
// session-authenticated site to the token-authenticated API.
func (a *API) IssueToken(ctx *http.Context) error {
	user, ok := http.UserAs[models.User](ctx)
	if !ok {
		return ctx.Unauthorized()
	}

	type request struct {
		Name      string   `json:"name" form:"name" validate:"required,max=64"`
		Abilities []string `json:"abilities" form:"abilities"`
	}

	req, err := http.ValidateRequest[request](ctx)
	if err != nil {
		return err
	}

	plaintext, token, err := a.Tokens.Create(user, req.Name, req.Abilities, nil)
	if err != nil {
		return err
	}

	// The plaintext is returned once and never again: the database holds
	// only its hash.
	return ctx.Created(map[string]any{
		"id":        token.ID,
		"name":      token.Name,
		"abilities": token.Abilities,
		"token":     plaintext,
	})
}
