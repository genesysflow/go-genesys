package console

import (
	"github.com/genesysflow/go-genesys/contracts"
	"github.com/spf13/cobra"
)

// Kernel is the console kernel that handles CLI commands.
type Kernel struct {
	app     contracts.Application
	rootCmd *cobra.Command
}

// KernelConfig defines configuration for the console kernel.
type KernelConfig struct {
	// Name is the application name used in the CLI.
	Name string

	// Short is a short description of the application.
	Short string

	// Long is a long description of the application.
	Long string
}

// DefaultKernelConfig returns the default kernel configuration.
func DefaultKernelConfig() KernelConfig {
	return KernelConfig{
		Name:  "app",
		Short: "Application CLI",
		Long:  "A web application built with the Go-Genesys framework.",
	}
}

// NewKernel creates a new console kernel.
func NewKernel(app contracts.Application, config ...KernelConfig) *Kernel {
	cfg := DefaultKernelConfig()
	if len(config) > 0 {
		cfg = config[0]
	}

	rootCmd := &cobra.Command{
		Use:   cfg.Name,
		Short: cfg.Short,
		Long:  cfg.Long,
	}

	return &Kernel{
		app:     app,
		rootCmd: rootCmd,
	}
}

// Run executes the console application with os.Args.
func (k *Kernel) Run() error {
	return k.rootCmd.Execute()
}

// Handle executes the console application with provided arguments.
func (k *Kernel) Handle(args []string) error {
	k.rootCmd.SetArgs(args)
	return k.rootCmd.Execute()
}

// RootCommand returns the underlying cobra root command.
func (k *Kernel) RootCommand() *cobra.Command {
	return k.rootCmd
}

// App returns the application instance for resolving dependencies.
func (k *Kernel) App() contracts.Application {
	return k.app
}

// AddCommand adds a command to the root command.
func (k *Kernel) AddCommand(cmds ...*cobra.Command) {
	k.rootCmd.AddCommand(cmds...)
}
