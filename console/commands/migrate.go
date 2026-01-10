package commands

import (
	"fmt"

	"github.com/genesysflow/go-genesys/container"
	"github.com/genesysflow/go-genesys/contracts"
	"github.com/genesysflow/go-genesys/database"
	"github.com/genesysflow/go-genesys/database/migrations"
	"github.com/genesysflow/go-genesys/database/schema"
	"github.com/spf13/cobra"
)

// MigrateCommand creates the migrate command.
func MigrateCommand(app contracts.Application) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Run database migrations",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := app.Boot(); err != nil {
				return fmt.Errorf("failed to boot application: %w", err)
			}

			migrator, err := container.Resolve[*migrations.Migrator](app)
			if err != nil {
				return fmt.Errorf("migrator not available: %w", err)
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

				// Auto-dump schema if requested
				if dump, _ := cmd.Flags().GetBool("dump-schema"); dump {
					// Resolve Database Manager
					mgr, err := container.Resolve[*database.Manager](app)
					if err != nil {
						fmt.Printf("Warning: could not resolve database manager for schema dump: %v\n", err)
					} else {
						conn := mgr.Connection()
						if conn == nil || conn.DB() == nil {
							fmt.Println("Warning: no database connection available for schema dump")
						} else {
							// Default driver from connection
							dumper := schema.NewDumper(conn.DB(), conn.Driver())
							// Default path
							path := "database/schema/schema.sql"
							if err := dumper.Dump(path); err != nil {
								fmt.Printf("Warning: failed to dump schema: %v\n", err)
							} else {
								fmt.Println("Schema dumped successfully.")
							}
						}
					}
				}
			}

			return nil
		},
	}

	cmd.Flags().Bool("dump-schema", true, "Dump schema after successful migration")

	return cmd
}

// MigrateRollbackCommand creates the migrate:rollback command.
func MigrateRollbackCommand(app contracts.Application) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate:rollback",
		Short: "Rollback the last database migration batch",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := app.Boot(); err != nil {
				return fmt.Errorf("failed to boot application: %w", err)
			}

			migrator, err := container.Resolve[*migrations.Migrator](app)
			if err != nil {
				return fmt.Errorf("migrator not available: %w", err)
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

				// Auto-dump schema if requested
				if dump, _ := cmd.Flags().GetBool("dump-schema"); dump {
					// Resolve Database Manager
					mgr, err := container.Resolve[*database.Manager](app)
					if err != nil {
						fmt.Printf("Warning: could not resolve database manager for schema dump: %v\n", err)
					} else {
						conn := mgr.Connection()
						if conn == nil || conn.DB() == nil {
							fmt.Println("Warning: no database connection available for schema dump")
						} else {
							// Default driver from connection
							dumper := schema.NewDumper(conn.DB(), conn.Driver())
							path := "database/schema/schema.sql"
							if err := dumper.Dump(path); err != nil {
								fmt.Printf("Warning: failed to dump schema: %v\n", err)
							} else {
								fmt.Println("Schema dumped successfully.")
							}
						}
					}
				}
			}

			return nil
		},
	}

	cmd.Flags().Bool("dump-schema", true, "Dump schema after successful rollback")

	return cmd
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

			migrator, err := container.Resolve[*migrations.Migrator](app)
			if err != nil {
				return fmt.Errorf("migrator not available: %w", err)
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
