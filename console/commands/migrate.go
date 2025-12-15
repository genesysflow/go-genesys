package commands

import (
	"fmt"

	"github.com/genesysflow/go-genesys/contracts"
	"github.com/genesysflow/go-genesys/database/migrations"
	"github.com/spf13/cobra"
)

// MigrateCommand creates the migrate command.
func MigrateCommand(app contracts.Application) *cobra.Command {
	return &cobra.Command{
		Use:   "migrate",
		Short: "Run database migrations",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := app.Boot(); err != nil {
				return fmt.Errorf("failed to boot application: %w", err)
			}

			migratorService, err := app.Make("migrator")
			if err != nil {
				return fmt.Errorf("migrator not available: %w", err)
			}

			migrator, ok := migratorService.(*migrations.Migrator)
			if !ok {
				return fmt.Errorf("migrator service is not *migrations.Migrator")
			}

			ran, err := migrator.Run()
			if err != nil {
				return err
			}

			if len(ran) == 0 {
				fmt.Println("Nothing to migrate.")
			} else {
				for _, name := range ran {
					fmt.Printf("Migrated: %s\n", name)
				}
			}

			return nil
		},
	}
}

// MigrateRollbackCommand creates the migrate:rollback command.
func MigrateRollbackCommand(app contracts.Application) *cobra.Command {
	return &cobra.Command{
		Use:   "migrate:rollback",
		Short: "Rollback the last database migration batch",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := app.Boot(); err != nil {
				return fmt.Errorf("failed to boot application: %w", err)
			}

			migratorService, err := app.Make("migrator")
			if err != nil {
				return fmt.Errorf("migrator not available: %w", err)
			}

			migrator, ok := migratorService.(*migrations.Migrator)
			if !ok {
				return fmt.Errorf("migrator service is not *migrations.Migrator")
			}

			rolledBack, err := migrator.Rollback()
			if err != nil {
				return err
			}

			if len(rolledBack) == 0 {
				fmt.Println("Nothing to rollback.")
			} else {
				for _, name := range rolledBack {
					fmt.Printf("Rolled back: %s\n", name)
				}
			}

			return nil
		},
	}
}

// MigrateStatusCommand creates the migrate:status command.
func MigrateStatusCommand(app contracts.Application) *cobra.Command {
	return &cobra.Command{
		Use:   "migrate:status",
		Short: "Show the status of each migration",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := app.Boot(); err != nil {
				return fmt.Errorf("failed to boot application: %w", err)
			}

			migratorService, err := app.Make("migrator")
			if err != nil {
				return fmt.Errorf("migrator not available: %w", err)
			}

			migrator, ok := migratorService.(*migrations.Migrator)
			if !ok {
				return fmt.Errorf("migrator service is not *migrations.Migrator")
			}

			status, err := migrator.Status()
			if err != nil {
				return err
			}

			if len(status) == 0 {
				fmt.Println("No migrations found.")
				return nil
			}

			fmt.Println("+------+--------------------------------------------------+-------+")
			fmt.Println("| Ran? | Migration                                        | Batch |")
			fmt.Println("+------+--------------------------------------------------+-------+")

			for _, s := range status {
				ran := "No"
				if s.Ran {
					ran = "Yes"
				}
				fmt.Printf("| %-4s | %-48s | %-5d |\n", ran, s.Name, s.Batch)
			}
			fmt.Println("+------+--------------------------------------------------+-------+")

			return nil
		},
	}
}
