package commands

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/genesysflow/go-genesys/contracts"
	"github.com/genesysflow/go-genesys/crypt"
	"github.com/spf13/cobra"
)

// KeyGenerateCommand creates the key:generate command.
func KeyGenerateCommand(app contracts.Application) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "key:generate",
		Short: "Generate a new application key and write it to .env",
		RunE: func(cmd *cobra.Command, args []string) error {
			key := crypt.GenerateKey()

			if show, _ := cmd.Flags().GetBool("show"); show {
				fmt.Println(key)
				return nil
			}

			envPath, _ := cmd.Flags().GetString("env")
			if err := writeAppKey(envPath, key); err != nil {
				return err
			}
			fmt.Println("Application key set successfully.")
			return nil
		},
	}

	cmd.Flags().Bool("show", false, "Display the key instead of writing it to .env")
	cmd.Flags().String("env", ".env", "Path to the .env file to update")

	return cmd
}

// writeAppKey sets APP_KEY in the env file, replacing an existing entry or
// appending one; the file is created when missing.
func writeAppKey(path, key string) error {
	line := "APP_KEY=" + key

	content, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		return os.WriteFile(path, []byte(line+"\n"), 0o600)
	}

	pattern := regexp.MustCompile(`(?m)^APP_KEY=.*$`)
	if pattern.Match(content) {
		return os.WriteFile(path, pattern.ReplaceAll(content, []byte(line)), 0o600)
	}

	text := string(content)
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	return os.WriteFile(path, []byte(text+line+"\n"), 0o600)
}
