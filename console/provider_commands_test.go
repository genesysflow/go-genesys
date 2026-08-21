package console_test

import (
	"testing"

	"github.com/genesysflow/go-genesys/console"
	"github.com/genesysflow/go-genesys/foundation"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A command the provider forgets to register does not exist as far as
// the user is concerned, so the registered set is asserted directly.
func TestProviderRegistersCommands(t *testing.T) {
	app := foundation.New()
	provider := &console.ConsoleServiceProvider{AppName: "test"}
	require.NoError(t, provider.Register(app))
	require.NoError(t, provider.Boot(app))

	kernel := provider.Kernel()
	require.NotNil(t, kernel)

	registered := map[string]bool{}
	var walk func(cmd *cobra.Command)
	walk = func(cmd *cobra.Command) {
		for _, child := range cmd.Commands() {
			registered[child.Name()] = true
			walk(child)
		}
	}
	walk(kernel.RootCommand())

	for _, name := range []string{
		"migrate", "queue:work", "route:list", "about",
		"cache:clear", "cache:forget", "storage:link", "config:show",
		"migrate:refresh", "db:wipe", "db:show", "db:table",
		"schedule:test", "event:list",
		"make:mail", "make:notification", "make:factory", "make:resource",
		"make:rule", "make:observer", "make:cast", "make:scope",
		"make:channel", "make:exception", "make:enum", "make:test",
		"make:view", "make:component",
	} {
		assert.True(t, registered[name], "command %q should be registered", name)
	}
}
