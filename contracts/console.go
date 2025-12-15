package contracts

import "github.com/spf13/cobra"

// Kernel defines the interface for the console kernel.
type Kernel interface {
	// Run executes the console application with os.Args.
	Run() error

	// Handle executes the console application with provided arguments.
	Handle(args []string) error

	// RootCommand returns the underlying cobra root command.
	RootCommand() *cobra.Command
}
