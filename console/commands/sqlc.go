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

			// Prepare args for sqlc: cli.Run expects arguments equivalent to os.Args[1:],
			// where the first element is the subcommand ("generate") followed by any additional arguments.
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
