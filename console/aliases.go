package console

import "github.com/genesysflow/go-genesys/console/cli"

// The command-authoring types live in console/cli so the framework's own
// commands can use them without importing this package, which registers
// them. They are aliased here because console is where an application
// reaches for them:
//
//	var Greet = &console.Command{Name: "greet", Handle: ...}
type (
	// Command describes a console command. See cli.Command.
	Command = cli.Command

	// Context is what a running command talks to the user through.
	// See cli.Context.
	Context = cli.Context

	// Argument is a positional argument. See cli.Argument.
	Argument = cli.Argument

	// Option is a flag. See cli.Option.
	Option = cli.Option
)
