package commands

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
)

// ServeCmd creates the 'serve' command.
func ServeCmd() *cobra.Command {
	var port string
	var host string

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run the development server",
		Long: `Start the development server for your Go-Genesys application.

Example:
  genesys serve
  genesys serve --port 8080
  genesys serve --host 0.0.0.0 --port 3000`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return serve(host, port)
		},
	}

	cmd.Flags().StringVarP(&port, "port", "p", "3000", "Port to run the server on")
	cmd.Flags().StringVarP(&host, "host", "H", "localhost", "Host to bind the server to")

	return cmd
}

func serve(host, port string) error {
	// Check if main.go exists
	if _, err := os.Stat("main.go"); os.IsNotExist(err) {
		return fmt.Errorf("main.go not found. Are you in a Go-Genesys project directory?")
	}

	// Set environment variables
	os.Setenv("PORT", port)
	os.Setenv("HOST", host)

	fmt.Printf("Starting development server at http://%s:%s\n", host, port)
	fmt.Println("Press Ctrl+C to stop")
	fmt.Println()

	// Run go run main.go
	goCmd := exec.Command("go", "run", "main.go")
	goCmd.Stdout = os.Stdout
	goCmd.Stderr = os.Stderr
	goCmd.Stdin = os.Stdin

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Start the command
	if err := goCmd.Start(); err != nil {
		return fmt.Errorf("failed to start server: %w", err)
	}

	// Wait for interrupt or command completion
	done := make(chan error, 1)
	go func() {
		done <- goCmd.Wait()
	}()

	select {
	case <-sigChan:
		fmt.Println("\nShutting down server...")
		if goCmd.Process != nil {
			goCmd.Process.Signal(os.Interrupt)
		}
		return nil
	case err := <-done:
		return err
	}
}
