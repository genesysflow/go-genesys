package commands_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/genesysflow/go-genesys/cache"
	"github.com/genesysflow/go-genesys/console/cli"
	"github.com/genesysflow/go-genesys/console/commands"
	"github.com/genesysflow/go-genesys/container"
	"github.com/genesysflow/go-genesys/contracts"
	"github.com/genesysflow/go-genesys/foundation"
	"github.com/genesysflow/go-genesys/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runCommand executes a command definition against app, capturing output.
func runCommand(t *testing.T, app contracts.Application, cmd *cli.Command, argv ...string) (string, error) {
	t.Helper()

	var out bytes.Buffer
	cobraCmd := cmd.Cobra(app)
	cobraCmd.SetOut(&out)
	cobraCmd.SetErr(&out)
	cobraCmd.SetArgs(argv)

	err := cobraCmd.Execute()
	return out.String(), err
}

// cacheApp boots an application with a memory cache store.
func cacheApp(t *testing.T) (*foundation.Application, cache.Store) {
	t.Helper()

	app := foundation.New()
	require.NoError(t, app.Register(&providers.CacheServiceProvider{}))
	require.NoError(t, app.Boot())

	manager := container.MustResolve[*cache.Manager](app)
	require.NotNil(t, manager)
	store, err := manager.Store()
	require.NoError(t, err)
	return app, store
}

func TestCacheClearCommand(t *testing.T) {
	app, store := cacheApp(t)
	require.NoError(t, store.Put("users:all", "cached", 0))

	out, err := runCommand(t, app, commands.CacheClearCommand(app))
	require.NoError(t, err)
	assert.Contains(t, out, "cleared")

	value, err := store.Get("users:all")
	require.NoError(t, err)
	assert.Nil(t, value, "the store should be empty after cache:clear")
}

func TestCacheForgetCommand(t *testing.T) {
	app, store := cacheApp(t)
	require.NoError(t, store.Put("keep", "yes", 0))
	require.NoError(t, store.Put("drop", "no", 0))

	out, err := runCommand(t, app, commands.CacheForgetCommand(app), "drop")
	require.NoError(t, err)
	assert.Contains(t, out, "drop")

	dropped, err := store.Get("drop")
	require.NoError(t, err)
	assert.Nil(t, dropped)

	kept, err := store.Get("keep")
	require.NoError(t, err)
	assert.Equal(t, "yes", kept)
}

// cache:forget needs a key: forgetting nothing silently would look like
// success.
func TestCacheForgetRequiresKey(t *testing.T) {
	app, _ := cacheApp(t)

	_, err := runCommand(t, app, commands.CacheForgetCommand(app))
	assert.Error(t, err)
}

// --- storage:link ----------------------------------------------------

func TestStorageLinkCommand(t *testing.T) {
	base := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(base, "storage", "app", "public"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(base, "public"), 0o755))

	app := foundation.New(base)
	require.NoError(t, app.Boot())

	out, err := runCommand(t, app, commands.StorageLinkCommand(app))
	require.NoError(t, err)
	assert.Contains(t, out, "public/storage")

	link := filepath.Join(base, "public", "storage")
	target, err := os.Readlink(link)
	require.NoError(t, err, "public/storage should be a symlink")
	assert.Contains(t, target, filepath.Join("storage", "app", "public"))
}

// The target directory is created when missing: a fresh checkout has no
// storage/app/public, and a link to nothing is not a working link.
func TestStorageLinkCreatesTarget(t *testing.T) {
	base := t.TempDir()
	app := foundation.New(base)
	require.NoError(t, app.Boot())

	_, err := runCommand(t, app, commands.StorageLinkCommand(app))
	require.NoError(t, err)

	info, err := os.Stat(filepath.Join(base, "storage", "app", "public"))
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

// Running twice is not an error, but an existing link is never replaced
// without --force: it may be a real directory holding uploads.
func TestStorageLinkIsIdempotentAndRefusesToClobber(t *testing.T) {
	base := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(base, "public"), 0o755))

	app := foundation.New(base)
	require.NoError(t, app.Boot())

	_, err := runCommand(t, app, commands.StorageLinkCommand(app))
	require.NoError(t, err)

	out, err := runCommand(t, app, commands.StorageLinkCommand(app))
	require.NoError(t, err, "re-linking the same target is a no-op")
	assert.Contains(t, out, "already")

	// A real directory in the way is refused.
	realDir := filepath.Join(base, "public", "storage")
	require.NoError(t, os.Remove(realDir))
	require.NoError(t, os.MkdirAll(realDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(realDir, "upload.txt"), []byte("data"), 0o600))

	_, err = runCommand(t, app, commands.StorageLinkCommand(app))
	require.Error(t, err, "an existing directory must not be replaced silently")

	_, err = os.Stat(filepath.Join(realDir, "upload.txt"))
	assert.NoError(t, err, "the existing directory must be left alone")
}

func TestStorageLinkForceReplacesExistingLink(t *testing.T) {
	base := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(base, "public"), 0o755))
	require.NoError(t, os.Symlink(t.TempDir(), filepath.Join(base, "public", "storage")))

	app := foundation.New(base)
	require.NoError(t, app.Boot())

	_, err := runCommand(t, app, commands.StorageLinkCommand(app), "--force")
	require.NoError(t, err)

	target, err := os.Readlink(filepath.Join(base, "public", "storage"))
	require.NoError(t, err)
	assert.Contains(t, target, filepath.Join("storage", "app", "public"))
}

// --- config:show -----------------------------------------------------

func TestConfigShowCommand(t *testing.T) {
	app := foundation.New()
	require.NoError(t, app.Boot())
	app.GetConfig().Set("mail.driver", "smtp")
	app.GetConfig().Set("mail.host", "smtp.example.com")

	out, err := runCommand(t, app, commands.ConfigShowCommand(app), "mail")
	require.NoError(t, err)
	assert.Contains(t, out, "mail.driver")
	assert.Contains(t, out, "smtp.example.com")
}

func TestConfigShowSingleValue(t *testing.T) {
	app := foundation.New()
	require.NoError(t, app.Boot())
	app.GetConfig().Set("app.name", "Genesys")

	out, err := runCommand(t, app, commands.ConfigShowCommand(app), "app.name")
	require.NoError(t, err)
	assert.Contains(t, out, "Genesys")
}

// A key that holds nothing is reported, not printed as an empty line the
// user has to squint at.
func TestConfigShowUnknownKey(t *testing.T) {
	app := foundation.New()
	require.NoError(t, app.Boot())

	out, err := runCommand(t, app, commands.ConfigShowCommand(app), "nope.missing")
	require.NoError(t, err)
	assert.Contains(t, out, "not set")
}

// Secrets must not be printed: config:show is the command people run in
// a screen-share.
func TestConfigShowMasksSecrets(t *testing.T) {
	app := foundation.New()
	require.NoError(t, app.Boot())
	app.GetConfig().Set("database.password", "super-secret")
	app.GetConfig().Set("database.host", "127.0.0.1")

	out, err := runCommand(t, app, commands.ConfigShowCommand(app), "database")
	require.NoError(t, err)
	assert.NotContains(t, out, "super-secret")
	assert.Contains(t, out, "127.0.0.1")

	out, err = runCommand(t, app, commands.ConfigShowCommand(app), "database", "--show-secrets")
	require.NoError(t, err)
	assert.Contains(t, out, "super-secret")
}
