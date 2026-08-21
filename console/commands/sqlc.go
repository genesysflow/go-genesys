package commands

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/genesysflow/go-genesys/contracts"
	"github.com/spf13/cobra"
)

// sqlcLookPath is swapped in tests.
var sqlcLookPath = exec.LookPath

// SqlcGenerateCommand creates the sqlc:generate command. It shells out
// to the sqlc binary rather than embedding the sqlc library, keeping
// the framework's dependency tree small (the embedded library dragged
// in a SQL parser, docker, grpc, and a wasm runtime).
func SqlcGenerateCommand(app contracts.Application) *cobra.Command {
	return &cobra.Command{
		Use: "sqlc:generate",
		// Pass every flag through to the sqlc binary untouched.
		DisableFlagParsing: true,
		Short:              "Generate type-safe database code using sqlc",
		Long: `Runs 'sqlc generate' using the sqlc binary from your PATH,
against the sqlc.yaml configuration in the current directory.

Install sqlc with:

  go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest

or see https://docs.sqlc.dev/en/latest/overview/install.html`,
		RunE: func(cmd *cobra.Command, args []string) error {
			binary, err := sqlcLookPath("sqlc")
			if err != nil {
				return fmt.Errorf("sqlc is not installed or not in PATH.\n\n" +
					"Install it with:\n\n" +
					"  go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest\n\n" +
					"or see https://docs.sqlc.dev/en/latest/overview/install.html")
			}

			fmt.Fprintln(cmd.OutOrStdout(), "Generating SQLC code...")

			sqlcArgs := append([]string{"generate"}, args...)
			generate := exec.Command(binary, sqlcArgs...)
			generate.Stdout = os.Stdout
			generate.Stderr = os.Stderr
			generate.Stdin = os.Stdin
			if err := generate.Run(); err != nil {
				return fmt.Errorf("sqlc generate failed: %w", err)
			}

			fmt.Fprintln(cmd.OutOrStdout(), "SQLC code generated successfully.")
			return nil
		},
	}
}
