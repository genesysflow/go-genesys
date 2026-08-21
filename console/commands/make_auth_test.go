package commands_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/genesysflow/go-genesys/console/commands"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMakeAuthScaffolding(t *testing.T) {
	app := generatorApp(t)

	out, err := runCommand(t, app, commands.MakeAuthCommand(app))
	require.NoError(t, err)
	assert.Contains(t, out, "auth")

	for _, path := range []string{
		"app/http/auth/controller.go",
		"app/http/auth/requests.go",
		"app/http/auth/routes.go",
		"resources/views/auth/login.html",
		"resources/views/auth/register.html",
		"resources/views/auth/forgot_password.html",
		"resources/views/auth/reset_password.html",
	} {
		_, statErr := os.Stat(filepath.Join(app.BasePath(), filepath.FromSlash(path)))
		assert.NoError(t, statErr, "%s should be generated", path)
	}
}

// The generated forms must carry the CSRF token and repopulate from old
// input, or the scaffolding teaches the wrong thing.
func TestMakeAuthViewsUseTheFrameworkHelpers(t *testing.T) {
	app := generatorApp(t)
	_, err := runCommand(t, app, commands.MakeAuthCommand(app))
	require.NoError(t, err)

	login, err := os.ReadFile(filepath.Join(app.BasePath(), "resources", "views", "auth", "login.html"))
	require.NoError(t, err)

	assert.Contains(t, string(login), "csrf_field")
	assert.Contains(t, string(login), `.old.Get "email"`)
	assert.Contains(t, string(login), `.errors.First "email"`)
	assert.Contains(t, string(login), `method="POST"`)

	// The password field must never be repopulated from old input.
	assert.NotContains(t, string(login), `.old.Get "password"`)
}

// Scaffolding must not overwrite an application's own work.
func TestMakeAuthRefusesToOverwrite(t *testing.T) {
	app := generatorApp(t)
	require.NoError(t, os.MkdirAll(filepath.Join(app.BasePath(), "app", "http", "auth"), 0o755))

	existing := filepath.Join(app.BasePath(), "app", "http", "auth", "controller.go")
	require.NoError(t, os.WriteFile(existing, []byte("package auth // mine"), 0o600))

	_, err := runCommand(t, app, commands.MakeAuthCommand(app))
	require.Error(t, err)

	content, readErr := os.ReadFile(existing)
	require.NoError(t, readErr)
	assert.Contains(t, string(content), "mine")
}

// The scaffolded controllers have to compile against the framework.
func TestMakeAuthGeneratesCompilingCode(t *testing.T) {
	if testing.Short() {
		t.Skip("compiling the scaffolding is slow")
	}

	app := generatorApp(t)
	_, err := runCommand(t, app, commands.MakeAuthCommand(app))
	require.NoError(t, err)

	root := repoRoot(t)
	workspace := filepath.Join(root, "console", "commands", "genauth")
	require.NoError(t, os.MkdirAll(workspace, 0o755))
	t.Cleanup(func() { _ = os.RemoveAll(workspace) })

	source := filepath.Join(app.BasePath(), "app", "http", "auth")
	entries, err := os.ReadDir(source)
	require.NoError(t, err)

	for _, entry := range entries {
		content, err := os.ReadFile(filepath.Join(source, entry.Name()))
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(workspace, entry.Name()), content, 0o600))
	}

	build := exec.Command("go", "build", "./console/commands/genauth/...")
	build.Dir = root
	output, err := build.CombinedOutput()
	assert.NoError(t, err, "the scaffolded auth code should compile:\n%s", output)
}
