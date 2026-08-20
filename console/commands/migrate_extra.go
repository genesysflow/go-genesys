package commands

import (
	"fmt"

	"github.com/genesysflow/go-genesys/container"
	"github.com/genesysflow/go-genesys/contracts"
	"github.com/genesysflow/go-genesys/database/migrations"
	"github.com/spf13/cobra"
)

func resolveMigrator(app contracts.Application) (*migrations.Migrator, error) {
	if err := app.Boot(); err != nil {
		return nil, fmt.Errorf("failed to boot application: %w", err)
	}
	migrator, err := container.Resolve[*migrations.Migrator](app)
	if err != nil {
		return nil, fmt.Errorf("migrator not available: %w", err)
	}
	return migrator, nil
}

// MigrateResetCommand creates the migrate:reset command.
func MigrateResetCommand(app contracts.Application) *cobra.Command {
	return &cobra.Command{
		Use:   "migrate:reset",
		Short: "Rollback all database migrations",
		RunE: func(cmd *cobra.Command, args []string) error {
			migrator, err := resolveMigrator(app)
			if err != nil {
				return err
			}
			rolledBack, err := migrator.Reset()
			if err != nil {
				return err
			}
			if len(rolledBack) == 0 {
				fmt.Println("Nothing to rollback.")
				return nil
			}
			for _, name := range rolledBack {
				fmt.Printf("Rolled back: %s\n", name)
			}
			return nil
		},
	}
}

// MigrateFreshCommand creates the migrate:fresh command.
func MigrateFreshCommand(app contracts.Application) *cobra.Command {
	return &cobra.Command{
		Use:   "migrate:fresh",
		Short: "Rollback all migrations and re-run them from scratch",
		RunE: func(cmd *cobra.Command, args []string) error {
			migrator, err := resolveMigrator(app)
			if err != nil {
				return err
			}

			if _, err := migrator.Reset(); err != nil {
				return fmt.Errorf("reset failed: %w", err)
			}
			ran, err := migrator.Run()
			if err != nil {
				return err
			}
			if len(ran) == 0 {
				fmt.Println("Nothing to migrate.")
				return nil
			}
			for _, name := range ran {
				fmt.Printf("Migrated: %s\n", name)
			}
			return nil
		},
	}
}
