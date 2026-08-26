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

	return ctx.CreatedResource(database.ToMap(post))
}

// Me describes the caller and the token they used.
func (a *API) Me(ctx *http.Context) error {
	user := models.CurrentUser(ctx)
	if user == nil {
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

// ShowTokens lists the caller's personal access tokens and offers the
// form that mints another. Without it the issuing endpoint is reachable
// only by someone who already knows it is there.
func (a *API) ShowTokens(ctx *http.Context) error {
	user := models.CurrentUser(ctx)
	if user == nil {
		return ctx.Unauthorized()
	}

	tokens, err := a.Tokens.ListFor(user)
	if err != nil {
		return err
	}

	// The abilities the API's routes actually check, so the form offers
	// choices that mean something rather than a free-text field.
	return ctx.View("tokens.index", map[string]any{
		"tokens":    tokens,
		"abilities": []string{"profile:read", "posts:write"},
	})
}

// IssueToken mints a token for the signed-in user - the bridge from the
// session-authenticated site to the token-authenticated API.
func (a *API) IssueToken(ctx *http.Context) error {
	user := models.CurrentUser(ctx)
	if user == nil {
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
	if ctx.IsJSON() {
		return ctx.Created(map[string]any{
			"id":        token.ID,
			"name":      token.Name,
			"abilities": token.Abilities,
			"token":     plaintext,
		})
	}

	// A browser gets the same one-time value where it can act on it: on
	// the token page, flashed for a single render.
	return ctx.RedirectToRoute("tokens.index").
		With("status", "Token created. Copy it now — it is not shown again.").
		With("new_token", plaintext).
		Send()
}

// RevokeToken destroys one of the caller's own tokens. The id arrives in
// the URL, where anyone may put any number, so the token is looked for
// among the caller's own before anything is deleted - an unparseable id
// is simply not among them.
//
// Someone else's token answers 404 rather than 403, because a 403
// confirms the id exists, which is the one thing a caller walking the id
// space is trying to learn.
func (a *API) RevokeToken(ctx *http.Context) error {
	user := models.CurrentUser(ctx)
	if user == nil {
		return ctx.Unauthorized()
	}

	tokens, err := a.Tokens.ListFor(user)
	if err != nil {
		return err
	}

	id := int64(ctx.ParamInt("token"))
	if !owns(tokens, id) {
		return ctx.NotFound()
	}

	if err := a.Tokens.Revoke(id); err != nil {
		return err
	}

	if ctx.IsJSON() {
		return ctx.NoContent()
	}

	return ctx.RedirectToRoute("tokens.index").
		With("status", "Token revoked.").
		Send()
}

// owns reports whether an id is among the tokens a caller holds. It is
// what stands between a token id in a URL and someone else's API access.
func owns(tokens []genesysauth.PersonalAccessToken, id int64) bool {
	for _, token := range tokens {
		if token.ID == id {
			return true
		}
	}
	return false
}
