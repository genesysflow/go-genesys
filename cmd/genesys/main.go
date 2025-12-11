// Package main provides the genesys CLI tool.
package main

import (
	"fmt"
	"os"

	"github.com/genesysflow/go-genesys/cmd/genesys/commands"
	"github.com/spf13/cobra"
)

var version = "1.0.0"

func main() {
	rootCmd := &cobra.Command{
		Use:   "genesys",
		Short: "Go-Genesys CLI - A Laravel-inspired Go web framework",
		Long: `Go-Genesys CLI provides tools for creating and managing
Go web applications using the Go-Genesys framework.

Inspired by Laravel's elegant syntax and powerful features.`,
		Version: version,
	}

	// Add commands
	rootCmd.AddCommand(commands.NewCmd())
	rootCmd.AddCommand(commands.MakeProviderCmd())
	rootCmd.AddCommand(commands.MakeControllerCmd())
	rootCmd.AddCommand(commands.MakeMiddlewareCmd())
	rootCmd.AddCommand(commands.ServeCmd())

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
