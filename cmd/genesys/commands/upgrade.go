package commands

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/genesysflow/go-genesys/foundation"
	"github.com/spf13/cobra"
)

// UpgradeCmd creates the 'upgrade' command.
func UpgradeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Upgrade Go-Genesys framework to the latest version",
		Long: `Upgrade the Go-Genesys framework dependency in the current project
to the latest version.

Example:
  genesys upgrade`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUpgrade()
		},
	}

	return cmd
}

func runUpgrade() error {
	// Check if go.mod exists
	if _, err := os.Stat("go.mod"); os.IsNotExist(err) {
		return fmt.Errorf("go.mod not found. Are you in a Go-Genesys project directory?")
	}

	// Check if go-genesys is in go.mod
	content, err := os.ReadFile("go.mod")
	if err != nil {
		return fmt.Errorf("failed to read go.mod: %w", err)
	}
	if !strings.Contains(string(content), "github.com/genesysflow/go-genesys") {
		return fmt.Errorf("go-genesys dependency not found in go.mod")
	}

	fmt.Println("Upgrading Go-Genesys framework...")

	// Use go get to update the dependency
	version := foundation.Version
	if !strings.HasPrefix(version, "v") {
		version = "v" + version
	}

	getCmd := exec.Command("go", "get", "github.com/genesysflow/go-genesys@"+version)
	getCmd.Stdout = os.Stdout
	getCmd.Stderr = os.Stderr
	if err := getCmd.Run(); err != nil {
		return fmt.Errorf("failed to update dependency: %w", err)
	}

	// Run go mod tidy
	fmt.Println("Running go mod tidy...")
	tidyCmd := exec.Command("go", "mod", "tidy")
	tidyCmd.Stdout = os.Stdout
	tidyCmd.Stderr = os.Stderr
	if err := tidyCmd.Run(); err != nil {
		return fmt.Errorf("go mod tidy failed: %w", err)
	}

	fmt.Printf("\n✓ Upgraded to Go-Genesys %s\n", version)
	return nil
}
