package commands_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/genesysflow/go-genesys/console/cli"
	"github.com/genesysflow/go-genesys/console/commands"
	"github.com/genesysflow/go-genesys/contracts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// goGenerators are the make:* commands that emit Go, with the directory
// each writes into. A stub that does not compile is worse than no stub:
// the developer's first act is to debug the framework's own output.
var goGenerators = []struct {
	name string
	dir  string
	make func(contracts.Application) *cli.Command
}{
	{"make:mail", "app/mail", commands.MakeMailCommand},
	{"make:notification", "app/notifications", commands.MakeNotificationCommand},
	{"make:factory", "database/factories", commands.MakeFactoryCommand},
	{"make:resource", "app/resources", commands.MakeResourceCommand},
	{"make:rule", "app/rules", commands.MakeRuleCommand},
	{"make:observer", "app/observers", commands.MakeObserverCommand},
	{"make:cast", "app/casts", commands.MakeCastCommand},
	{"make:scope", "app/scopes", commands.MakeScopeCommand},
	{"make:channel", "app/broadcasting", commands.MakeChannelCommand},
	{"make:exception", "app/exceptions", commands.MakeExceptionCommand},
	{"make:enum", "app/enums", commands.MakeEnumCommand},
	{"make:policy", "app/policies", commands.MakePolicyCommand},
}

// Every Go stub a generator writes is compiled against the framework.
// Each goes into its own package directory, since they declare different
// package names and would otherwise collide.
func TestEveryGeneratorProducesCompilingCode(t *testing.T) {
	if testing.Short() {
		t.Skip("compiling every stub is slow")
	}

	root := repoRoot(t)
	workspace := filepath.Join(root, "console", "commands", "genstubs")
	require.NoError(t, os.MkdirAll(workspace, 0o755))
	t.Cleanup(func() { _ = os.RemoveAll(workspace) })

	for _, generator := range goGenerators {
		app := generatorApp(t)

		_, err := runCommand(t, app, generator.make(app), "Sample")
		require.NoError(t, err, "%s should generate", generator.name)

		source := filepath.Join(app.BasePath(), filepath.FromSlash(generator.dir))
		entries, err := os.ReadDir(source)
		require.NoError(t, err, "%s wrote nothing into %s", generator.name, generator.dir)

		// One directory per generator: the stubs declare different
		// package names and cannot share a directory.
		target := filepath.Join(workspace, filepath.Base(generator.dir))
		require.NoError(t, os.MkdirAll(target, 0o755))

		for _, entry := range entries {
			content, err := os.ReadFile(filepath.Join(source, entry.Name()))
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filepath.Join(target, entry.Name()), content, 0o600))
		}
	}

	build := exec.Command("go", "build", "./console/commands/genstubs/...")
	build.Dir = root
	output, err := build.CombinedOutput()
	assert.NoError(t, err, "every generated stub should compile:\n%s", output)
}
