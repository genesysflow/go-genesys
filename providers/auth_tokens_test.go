package providers_test

import (
	"testing"

	"github.com/genesysflow/go-genesys/auth"
	"github.com/genesysflow/go-genesys/container"
	"github.com/genesysflow/go-genesys/database"
	"github.com/genesysflow/go-genesys/database/schema"
	"github.com/genesysflow/go-genesys/foundation"
	"github.com/genesysflow/go-genesys/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

type tokenUser struct {
	database.Model
	Email string `db:"email"`
}

func (u *tokenUser) GetAuthIdentifier() any  { return u.ID }
func (u *tokenUser) GetAuthPassword() string { return "" }

// Personal access tokens are only usable if the provider hands the
// application a repository wired to its database.
func TestAuthProviderRegistersTokenRepository(t *testing.T) {
	app := foundation.New()
	require.NoError(t, app.Register(&providers.DatabaseServiceProvider{
		Config: &database.Config{
			Default: "test",
			Connections: map[string]database.ConnectionConfig{
				"test": {Driver: "sqlite", Database: ":memory:"},
			},
		},
	}))
	require.NoError(t, app.Register(&providers.AuthServiceProvider{
		UserProvider: auth.NewORMUserProvider[tokenUser](),
	}))
	require.NoError(t, app.Boot())

	conn := container.MustResolve[*database.Manager](app).Connection()
	require.NoError(t, auth.CreatePersonalAccessTokensTable(schema.NewBuilder(conn.DB(), "sqlite")))

	repo, err := container.Resolve[*auth.TokenRepository](app)
	require.NoError(t, err, "the auth provider should register a token repository")

	plaintext, _, err := repo.Create(&tokenUser{Model: database.Model{ID: 1}}, "cli", nil, nil)
	require.NoError(t, err)

	found, err := repo.Find(plaintext)
	require.NoError(t, err)
	assert.Equal(t, int64(1), found.TokenableID)
}
