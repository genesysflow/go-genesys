package commands

import (
	"fmt"

	"github.com/genesysflow/go-genesys/container"
	"github.com/genesysflow/go-genesys/contracts"
	"github.com/genesysflow/go-genesys/database/seed"
	"github.com/spf13/cobra"
)

// DbSeedCommand creates the db:seed command.
func DbSeedCommand(app contracts.Application) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "db:seed",
		Short: "Seed the database with records",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := app.Boot(); err != nil {
				return fmt.Errorf("failed to boot application: %w", err)
			}
			runner, err := container.Resolve[*seed.Runner](app)
			if err != nil {
				return fmt.Errorf("seeder not available - register the SeedServiceProvider: %w", err)
			}

			names, _ := cmd.Flags().GetStringSlice("seeder")
			if err := runner.Run(names...); err != nil {
				return err
			}

			if len(names) > 0 {
				fmt.Printf("Seeded: %v\n", names)
			} else {
				fmt.Printf("Seeded: %v\n", runner.Names())
			}
			return nil
		},
	}

	cmd.Flags().StringSlice("seeder", nil, "Run only the named seeders")
	return cmd
}
