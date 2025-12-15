package commands

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"regexp"
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

	fmt.Println("Upgrading Go-Genesys framework...")

	// Read go.mod
	file, err := os.Open("go.mod")
	if err != nil {
		return fmt.Errorf("failed to open go.mod: %w", err)
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	genesysPattern := regexp.MustCompile(`github\.com/genesysflow/go-genesys`)
	found := false

	for scanner.Scan() {
		line := scanner.Text()
		if genesysPattern.MatchString(line) {
			// Update to latest version
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				oldVersion := "unknown"
				if len(parts) >= 2 {
					oldVersion = parts[len(parts)-1]
				}
				newLine := fmt.Sprintf("\tgithub.com/genesysflow/go-genesys v%s", foundation.Version)
				lines = append(lines, newLine)
				fmt.Printf("  Updating from %s to v%s\n", oldVersion, foundation.Version)
				found = true
				continue
			}
		}
		lines = append(lines, line)
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("failed to read go.mod: %w", err)
	}

	if !found {
		return fmt.Errorf("go-genesys dependency not found in go.mod")
	}

	// Write updated go.mod
	if err := os.WriteFile("go.mod", []byte(strings.Join(lines, "\n")+"\n"), 0644); err != nil {
		return fmt.Errorf("failed to write go.mod: %w", err)
	}

	// Run go mod tidy
	fmt.Println("Running go mod tidy...")
	tidyCmd := exec.Command("go", "mod", "tidy")
	tidyCmd.Stdout = os.Stdout
	tidyCmd.Stderr = os.Stderr
	if err := tidyCmd.Run(); err != nil {
		return fmt.Errorf("go mod tidy failed: %w", err)
	}

	fmt.Printf("\n✓ Upgraded to Go-Genesys v%s\n", foundation.Version)
	return nil
}
