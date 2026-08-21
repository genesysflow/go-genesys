package commands

import (
	"fmt"
	"runtime"

	"github.com/genesysflow/go-genesys/contracts"
	"github.com/genesysflow/go-genesys/env"
	"github.com/genesysflow/go-genesys/foundation"
	"github.com/spf13/cobra"
)

// AboutCommand creates the about command, summarising the application
// environment like Laravel's `artisan about`.
func AboutCommand(app contracts.Application) *cobra.Command {
	return &cobra.Command{
		Use:   "about",
		Short: "Display basic information about the application",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := app.Boot(); err != nil {
				return fmt.Errorf("failed to boot application: %w", err)
			}
			cfg := app.GetConfig()

			appName := cfg.GetString("app.name")
			if appName == "" {
				appName = "Go-Genesys Application"
			}
			appEnv := cfg.GetString("app.env")
			if appEnv == "" {
				appEnv = env.Get("APP_ENV", "local")
			}

			rows := [][2]string{
				{"Application", appName},
				{"Environment", appEnv},
				{"Debug", fmt.Sprint(cfg.GetBool("app.debug"))},
				{"URL", cfg.GetString("app.url")},
				{"Framework", "go-genesys " + foundation.Version},
				{"Go", runtime.Version()},
				{"OS/Arch", runtime.GOOS + "/" + runtime.GOARCH},
				{"Base path", app.BasePath()},
				{"Database", cfg.GetString("database.default")},
				{"Session driver", cfg.GetString("session.driver")},
				{"Cache store", cfg.GetString("cache.default")},
				{"Queue connection", cfg.GetString("queue.default")},
				{"Mail driver", cfg.GetString("mail.driver")},
			}

			for _, row := range rows {
				value := row[1]
				if value == "" {
					value = "-"
				}
				fmt.Printf("%-18s %s\n", row[0], value)
			}
			return nil
		},
	}
}
