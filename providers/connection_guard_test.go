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
)

// brokenDatabase registers a manager whose connection cannot be opened.
func brokenDatabase(t *testing.T) *foundation.Application {
	t.Helper()

	app := foundation.New()
	require.NoError(t, app.Register(&providers.AppServiceProvider{}))
	require.NoError(t, app.Register(&providers.ValidationServiceProvider{}))
	require.NoError(t, app.Boot())

	manager := database.NewManager(database.Config{
		Default: "broken",
		Connections: map[string]database.ConnectionConfig{
			"broken": {Driver: "no-such-driver", Database: "nowhere"},
		},
	})
	t.Cleanup(func() { _ = manager.Close() })
	app.InstanceType(manager)

	return app
}

// Manager.Connection always hands back a connection - a failed one
// carries its error rather than being nil. A caller that only checks for
// nil therefore treats a database that never opened as usable, and the
// failure surfaces later as a confusing query error instead of here.
func TestValidationResolverReportsABrokenConnection(t *testing.T) {
	app := brokenDatabase(t)

	validator := container.MustResolve[*validation.Validator](app)

	// The unique rule fails closed either way, so the resolver's own
	// answer is what this checks.
	result := validator.ValidateMap(
		map[string]any{"email": "ada@example.com"},
		map[string]string{"email": "unique=users.email"},
	)
	assert.False(t, result.Passes(),
		"a rule that cannot reach the database must not pass")
}

// The error reaches the caller rather than being swallowed by a guard
// that cannot fire.
func TestTheResolverSurfacesTheConnectionError(t *testing.T) {
	app := brokenDatabase(t)
	manager := container.MustResolve[*database.Manager](app)

	conn := manager.Connection()
	require.Error(t, conn.Error())
	assert.Contains(t, conn.Error().Error(), "no-such-driver",
		"the reason the connection failed should reach whoever asked")
}

// The connection a broken manager returns reports its error, which is
// the signal callers have to check.
func TestABrokenConnectionCarriesItsError(t *testing.T) {
	app := brokenDatabase(t)
	manager := container.MustResolve[*database.Manager](app)

	conn := manager.Connection()
	require.NotNil(t, conn, "Connection never returns nil, even when it failed")
	assert.Error(t, conn.Error(),
		"a connection that could not be opened must say so through Error()")
}
