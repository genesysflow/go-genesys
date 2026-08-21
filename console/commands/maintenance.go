package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/genesysflow/go-genesys/contracts"
	"github.com/genesysflow/go-genesys/http/middleware"
	"github.com/spf13/cobra"
)

func downFilePath(app contracts.Application, override string) string {
	if override != "" {
		return override
	}
	return filepath.Join(app.BasePath(), middleware.DefaultDownFilePath)
}

// DownCommand puts the application into maintenance mode.
func DownCommand(app contracts.Application) *cobra.Command {
	var message, secret, file string
	var retryAfter int

	cmd := &cobra.Command{
		Use:   "down",
		Short: "Put the application into maintenance mode",
		RunE: func(cmd *cobra.Command, args []string) error {
			path := downFilePath(app, file)
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return err
			}
			payload, err := json.MarshalIndent(middleware.MaintenancePayload{
				Message:    message,
				RetryAfter: retryAfter,
				Secret:     secret,
			}, "", "  ")
			if err != nil {
				return err
			}
			if err := os.WriteFile(path, payload, 0o644); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Application is now in maintenance mode.")
			if secret != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "Bypass with ?secret=%s\n", secret)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&message, "message", "", "Message shown to visitors")
	cmd.Flags().StringVar(&secret, "secret", "", "Secret that bypasses maintenance mode")
	cmd.Flags().IntVar(&retryAfter, "retry", 0, "Retry-After header value in seconds")
	cmd.Flags().StringVar(&file, "file", "", "Down file path override")
	return cmd
}

// UpCommand brings the application out of maintenance mode.
func UpCommand(app contracts.Application) *cobra.Command {
	var file string

	cmd := &cobra.Command{
		Use:   "up",
		Short: "Bring the application out of maintenance mode",
		RunE: func(cmd *cobra.Command, args []string) error {
			path := downFilePath(app, file)
			if err := os.Remove(path); err != nil {
				if os.IsNotExist(err) {
					fmt.Fprintln(cmd.OutOrStdout(), "Application is already up.")
					return nil
				}
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Application is now live.")
			return nil
		},
	}
	cmd.Flags().StringVar(&file, "file", "", "Down file path override")
	return cmd
}
