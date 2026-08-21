package auth_test

import (
	"database/sql"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/genesysflow/go-genesys/auth"
	"github.com/genesysflow/go-genesys/database"
	"github.com/genesysflow/go-genesys/database/schema"
	genhttp "github.com/genesysflow/go-genesys/http"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

// tokenRepo returns a repository over a fresh sqlite database.
func tokenRepo(t *testing.T) (*auth.TokenRepository, *sql.DB) {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	builder := schema.NewBuilder(db, "sqlite")
	require.NoError(t, auth.CreatePersonalAccessTokensTable(builder))

	return auth.NewTokenRepository("sqlite", db, ""), db
}

func tokenUser(id int64) *User {
	return &User{Model: database.Model{ID: id}}
}

func TestCreateAndFindToken(t *testing.T) {
	repo, _ := tokenRepo(t)
	user := tokenUser(1)

	plaintext, token, err := repo.Create(user, "cli", nil, nil)
	require.NoError(t, err)
	require.NotEmpty(t, plaintext)
	assert.Equal(t, "cli", token.Name)

	found, err := repo.Find(plaintext)
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, token.ID, found.ID)
	assert.Equal(t, int64(1), found.TokenableID)
}

// The plaintext token is never stored: a leaked database must not hand
// the attacker working credentials.
func TestTokenIsStoredHashed(t *testing.T) {
	repo, db := tokenRepo(t)

	plaintext, _, err := repo.Create(tokenUser(1), "cli", nil, nil)
	require.NoError(t, err)

	var stored string
	require.NoError(t, db.QueryRow("SELECT token FROM personal_access_tokens").Scan(&stored))

	assert.NotEqual(t, plaintext, stored)
	assert.NotContains(t, plaintext, stored)
	// The random half of the plaintext must not appear in the column.
	secret := plaintext[strings.Index(plaintext, "|")+1:]
	assert.NotContains(t, stored, secret)
}

func TestFindRejectsUnknownAndTamperedTokens(t *testing.T) {
	repo, _ := tokenRepo(t)

	plaintext, _, err := repo.Create(tokenUser(1), "cli", nil, nil)
	require.NoError(t, err)

	_, err = repo.Find("nonsense")
	assert.Error(t, err)

	_, err = repo.Find("999|" + strings.SplitN(plaintext, "|", 2)[1])
	assert.Error(t, err, "an id that does not own this secret must not authenticate")

	_, err = repo.Find(strings.SplitN(plaintext, "|", 2)[0] + "|wrong-secret")
	assert.Error(t, err)
}

func TestExpiredTokenIsRejected(t *testing.T) {
	repo, _ := tokenRepo(t)

	past := time.Now().Add(-time.Hour)
	plaintext, _, err := repo.Create(tokenUser(1), "cli", nil, &past)
	require.NoError(t, err)

	_, err = repo.Find(plaintext)
	assert.ErrorIs(t, err, auth.ErrTokenExpired)

	future := time.Now().Add(time.Hour)
	plaintext, _, err = repo.Create(tokenUser(1), "cli", nil, &future)
	require.NoError(t, err)
	_, err = repo.Find(plaintext)
	assert.NoError(t, err)
}

func TestTokenAbilities(t *testing.T) {
	repo, _ := tokenRepo(t)

	plaintext, _, err := repo.Create(tokenUser(1), "ci", []string{"posts:read", "posts:write"}, nil)
	require.NoError(t, err)

	token, err := repo.Find(plaintext)
	require.NoError(t, err)

	assert.True(t, token.Can("posts:read"))
	assert.True(t, token.Can("posts:write"))
	assert.False(t, token.Can("users:delete"))
	assert.True(t, token.Cannot("users:delete"))
}

// A token created with no abilities may do anything its user may do,
// matching Sanctum's "*".
func TestTokenWildcardAbility(t *testing.T) {
	repo, _ := tokenRepo(t)

	plaintext, _, err := repo.Create(tokenUser(1), "root", []string{"*"}, nil)
	require.NoError(t, err)

	token, err := repo.Find(plaintext)
	require.NoError(t, err)
	assert.True(t, token.Can("anything"))
}

// A token with an explicit ability list must not be widened by accident.
func TestTokenWithoutAbilitiesGrantsNothing(t *testing.T) {
	repo, _ := tokenRepo(t)

	plaintext, _, err := repo.Create(tokenUser(1), "scoped", []string{"posts:read"}, nil)
	require.NoError(t, err)

	token, err := repo.Find(plaintext)
	require.NoError(t, err)
	assert.False(t, token.Can("*"))
	assert.False(t, token.Can("posts:write"))
}

func TestRevokeToken(t *testing.T) {
	repo, _ := tokenRepo(t)

	plaintext, token, err := repo.Create(tokenUser(1), "cli", nil, nil)
	require.NoError(t, err)

	require.NoError(t, repo.Revoke(token.ID))

	_, err = repo.Find(plaintext)
	assert.Error(t, err)
}

func TestRevokeAllForUser(t *testing.T) {
	repo, _ := tokenRepo(t)

	first, _, err := repo.Create(tokenUser(1), "one", nil, nil)
	require.NoError(t, err)
	second, _, err := repo.Create(tokenUser(1), "two", nil, nil)
	require.NoError(t, err)
	other, _, err := repo.Create(tokenUser(2), "other", nil, nil)
	require.NoError(t, err)

	removed, err := repo.RevokeAll(tokenUser(1))
	require.NoError(t, err)
	assert.Equal(t, int64(2), removed)

	_, err = repo.Find(first)
	assert.Error(t, err)
	_, err = repo.Find(second)
	assert.Error(t, err)

	_, err = repo.Find(other)
	assert.NoError(t, err, "another user's tokens must survive")
}

func TestListTokensForUser(t *testing.T) {
	repo, _ := tokenRepo(t)

	_, _, err := repo.Create(tokenUser(1), "one", nil, nil)
	require.NoError(t, err)
	_, _, err = repo.Create(tokenUser(2), "other", nil, nil)
	require.NoError(t, err)

	tokens, err := repo.ListFor(tokenUser(1))
	require.NoError(t, err)
	require.Len(t, tokens, 1)
	assert.Equal(t, "one", tokens[0].Name)
}

// Using a token records when it was last used, so a stale token can be
// found and revoked.
func TestFindRecordsLastUsedAt(t *testing.T) {
	repo, _ := tokenRepo(t)

	plaintext, _, err := repo.Create(tokenUser(1), "cli", nil, nil)
	require.NoError(t, err)

	before, err := repo.ListFor(tokenUser(1))
	require.NoError(t, err)
	require.Nil(t, before[0].LastUsedAt)

	_, err = repo.Find(plaintext)
	require.NoError(t, err)

	after, err := repo.ListFor(tokenUser(1))
	require.NoError(t, err)
	require.NotNil(t, after[0].LastUsedAt)
}

func TestPruneExpiredTokens(t *testing.T) {
	repo, _ := tokenRepo(t)

	past := time.Now().Add(-time.Hour)
	_, _, err := repo.Create(tokenUser(1), "stale", nil, &past)
	require.NoError(t, err)
	_, _, err = repo.Create(tokenUser(1), "live", nil, nil)
	require.NoError(t, err)

	pruned, err := repo.PruneExpired()
	require.NoError(t, err)
	assert.Equal(t, int64(1), pruned)

	tokens, err := repo.ListFor(tokenUser(1))
	require.NoError(t, err)
	require.Len(t, tokens, 1)
	assert.Equal(t, "live", tokens[0].Name)
}

// --- guard -----------------------------------------------------------

func TestPersonalAccessTokenGuard(t *testing.T) {
	kernel, _, _ := setupAuthApp(t)
	repo, _ := tokenRepo(t)

	plaintext, _, err := repo.Create(tokenUser(1), "cli", []string{"posts:read"}, nil)
	require.NoError(t, err)

	guard := auth.NewPersonalAccessTokenGuard("api", repo, auth.NewORMUserProvider[User]())

	kernel.GET("/token-user", func(ctx *genhttp.Context) error {
		user := guard.User(ctx)
		require.NotNil(t, user)

		token := auth.TokenFrom(ctx)
		require.NotNil(t, token)
		assert.True(t, token.Can("posts:read"))

		return ctx.JSONResponse(map[string]any{"id": user.GetAuthIdentifier()})
	}, auth.Middleware(guard))

	req := httptest.NewRequest("GET", "/token-user", nil)
	req.Header.Set("Authorization", "Bearer "+plaintext)
	resp, err := kernel.Fiber().Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// A bad token is unauthenticated.
	req = httptest.NewRequest("GET", "/token-user", nil)
	req.Header.Set("Authorization", "Bearer 1|wrong")
	resp, err = kernel.Fiber().Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, 401, resp.StatusCode)
}

// A route may require an ability the token does not carry.
func TestRequireAbilityMiddleware(t *testing.T) {
	kernel, _, _ := setupAuthApp(t)
	repo, _ := tokenRepo(t)

	plaintext, _, err := repo.Create(tokenUser(1), "reader", []string{"posts:read"}, nil)
	require.NoError(t, err)

	guard := auth.NewPersonalAccessTokenGuard("api", repo, auth.NewORMUserProvider[User]())

	kernel.GET("/read", func(ctx *genhttp.Context) error {
		return ctx.String("ok")
	}, auth.Middleware(guard), auth.RequireAbility("posts:read"))

	kernel.DELETE("/posts/1", func(ctx *genhttp.Context) error {
		return ctx.String("deleted")
	}, auth.Middleware(guard), auth.RequireAbility("posts:delete"))

	req := httptest.NewRequest("GET", "/read", nil)
	req.Header.Set("Authorization", "Bearer "+plaintext)
	resp, err := kernel.Fiber().Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	req = httptest.NewRequest("DELETE", "/posts/1", nil)
	req.Header.Set("Authorization", "Bearer "+plaintext)
	resp, err = kernel.Fiber().Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, 403, resp.StatusCode)
}
