package commands

import (
	"fmt"

	"github.com/genesysflow/go-genesys/contracts"
	"github.com/spf13/cobra"
	"github.com/sqlc-dev/sqlc/pkg/cli"
)

// SqlcGenerateCommand creates the sqlc:generate command.
func SqlcGenerateCommand(app contracts.Application) *cobra.Command {
	return &cobra.Command{
		Use:   "sqlc:generate",
		Short: "Generate type-safe database code using embedded SQLC",
		Long: `This command runs 'sqlc generate' using the embedded SQLC library.
It looks for the 'sqlc.yaml' configuration file in the current directory.
It does not require 'sqlc' to be installed in your PATH.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("Generating SQLC code (embedded)...")

			// Prepare args for sqlc
			// cli.Run expects the full argument list including the command name if it parses os.Args style,
			// but typically library calls might expect just the args.
			// However, looking at widespread usage of such wrappers, they often mimic os.Args.
			// Let's try passing "generate" as the argument.
			sqlcArgs := []string{"generate"}
			sqlcArgs = append(sqlcArgs, args...)

			exitCode := cli.Run(sqlcArgs)
			if exitCode != 0 {
				return fmt.Errorf("sqlc generate failed with exit code %d", exitCode)
			}

			fmt.Println("✅ SQLC code generated successfully.")
			return nil
		},
	}
}
