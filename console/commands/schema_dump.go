package commands

import (
	"fmt"

	"github.com/genesysflow/go-genesys/container"
	"github.com/genesysflow/go-genesys/contracts"
	"github.com/genesysflow/go-genesys/database"
	"github.com/genesysflow/go-genesys/database/schema"
	"github.com/spf13/cobra"
)

// DbSchemaDumpCommand creates the db:schema:dump command.
func DbSchemaDumpCommand(app contracts.Application) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "db:schema:dump",
		Short: "Dump the database schema to a file",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := app.Boot(); err != nil {
				return fmt.Errorf("failed to boot application: %w", err)
			}

			mgr, err := container.Resolve[*database.Manager](app)
			if err != nil {
				return fmt.Errorf("database manager not available: %w", err)
			}

			conn := mgr.Connection()
			if conn == nil || conn.DB() == nil {
				return fmt.Errorf("no database connection available")
			}

			// Default schema path
			path, _ := cmd.Flags().GetString("path")

			dumper := schema.NewDumper(conn.DB(), conn.Driver())
			if err := dumper.Dump(path); err != nil {
				return fmt.Errorf("failed to dump schema: %w", err)
			}

			fmt.Printf("Schema dumped to %s\n", path)
			return nil
		},
	}

	cmd.Flags().String("path", "database/schema/schema.sql", "Path to the schema dump file")

	return cmd
}
