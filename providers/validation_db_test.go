package providers_test

import (
	"testing"

	"github.com/genesysflow/go-genesys/container"
	"github.com/genesysflow/go-genesys/database"
	"github.com/genesysflow/go-genesys/foundation"
	"github.com/genesysflow/go-genesys/providers"
	"github.com/genesysflow/go-genesys/validation"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

// The unique/exists rules must work out of the box once a database
// connection is configured: wiring them by hand in every app is exactly
// the boilerplate the provider exists to remove.
func TestValidationProviderWiresDatabaseRules(t *testing.T) {
	app := foundation.New()
	require.NoError(t, app.Register(&providers.DatabaseServiceProvider{
		Config: &database.Config{
			Default: "test",
			Connections: map[string]database.ConnectionConfig{
				"test": {Driver: "sqlite", Database: ":memory:"},
			},
		},
	}))
	require.NoError(t, app.Register(&providers.ValidationServiceProvider{}))
	require.NoError(t, app.Boot())

	conn := container.MustResolve[*database.Manager](app).Connection()
	_, err := conn.Exec(`CREATE TABLE users (id INTEGER PRIMARY KEY, email TEXT)`)
	require.NoError(t, err)
	_, err = conn.Exec(`INSERT INTO users (id, email) VALUES (1, 'ada@example.com')`)
	require.NoError(t, err)

	v := container.MustResolve[*validation.Validator](app)

	taken := v.ValidateMap(
		map[string]any{"email": "ada@example.com"},
		map[string]string{"email": "unique=users.email"},
	)
	assert.True(t, taken.Fails(), "an existing email is not unique")

	free := v.ValidateMap(
		map[string]any{"email": "grace@example.com"},
		map[string]string{"email": "unique=users.email"},
	)
	assert.True(t, free.Passes(), free.All())
}
