package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
)

// MakeProviderCmd creates the 'make:provider' command.
func MakeProviderCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "make:provider <name>",
		Short: "Create a new service provider",
		Long: `Create a new service provider class.

Example:
  genesys make:provider Cache
  genesys make:provider Payment`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			return makeProvider(name)
		},
	}

	return cmd
}

func makeProvider(name string) error {
	// Ensure name ends with ServiceProvider or just use as-is
	providerName := toPascalCase(name)
	if !strings.HasSuffix(providerName, "ServiceProvider") {
		providerName += "ServiceProvider"
	}

	// Create provider file
	dir := "app/providers"
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	filename := strings.ToLower(strings.TrimSuffix(providerName, "ServiceProvider")) + "_provider.go"
	path := filepath.Join(dir, filename)

	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("provider already exists: %s", path)
	}

	data := TemplateData{
		Name:      strings.TrimSuffix(providerName, "ServiceProvider"),
		Package:   "providers",
		LowerName: strings.ToLower(strings.TrimSuffix(providerName, "ServiceProvider")),
	}

	tmplContent, err := loadTemplate("provider.go.tmpl")
	if err != nil {
		return err
	}

	if err := generateFile(path, tmplContent, data); err != nil {
		return err
	}

	fmt.Printf("✓ Provider created: %s\n", path)
	return nil
}

// MakeControllerCmd creates the 'make:controller' command.
func MakeControllerCmd() *cobra.Command {
	var resource bool

	cmd := &cobra.Command{
		Use:   "make:controller <name>",
		Short: "Create a new controller",
		Long: `Create a new HTTP controller class.

Example:
  genesys make:controller User
  genesys make:controller User --resource`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			return makeController(name, resource)
		},
	}

	cmd.Flags().BoolVarP(&resource, "resource", "r", false, "Generate a resource controller")

	return cmd
}

func makeController(name string, resource bool) error {
	// Ensure name ends with Controller or just use as-is
	controllerName := toPascalCase(name)
	if !strings.HasSuffix(controllerName, "Controller") {
		controllerName += "Controller"
	}

	// Create controller file
	dir := "app/controllers"
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	filename := strings.ToLower(strings.TrimSuffix(controllerName, "Controller")) + "_controller.go"
	path := filepath.Join(dir, filename)

	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("controller already exists: %s", path)
	}

	baseName := strings.TrimSuffix(controllerName, "Controller")
	data := TemplateData{
		Name:      baseName,
		Package:   "controllers",
		LowerName: strings.ToLower(baseName),
		RouteName: toSnakeCase(baseName),
	}

	var tmplFilename string
	if resource {
		tmplFilename = "controller.go.tmpl"
	} else {
		tmplFilename = "controller_simple.go.tmpl"
	}

	tmplContent, err := loadTemplate(tmplFilename)
	if err != nil {
		return err
	}

	if err := generateFile(path, tmplContent, data); err != nil {
		return err
	}

	fmt.Printf("✓ Controller created: %s\n", path)
	return nil
}

// MakeMiddlewareCmd creates the 'make:middleware' command.
func MakeMiddlewareCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "make:middleware <name>",
		Short: "Create a new middleware",
		Long: `Create a new middleware class.

Example:
  genesys make:middleware Auth
  genesys make:middleware RateLimit`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			return makeMiddleware(name)
		},
	}

	return cmd
}

func makeMiddleware(name string) error {
	middlewareName := toPascalCase(name)
	middlewareName = strings.TrimSuffix(middlewareName, "Middleware")

	// Create middleware file
	dir := "app/middleware"
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	filename := toSnakeCase(middlewareName) + ".go"
	path := filepath.Join(dir, filename)

	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("middleware already exists: %s", path)
	}

	data := TemplateData{
		Name:      middlewareName,
		Package:   "middleware",
		LowerName: strings.ToLower(middlewareName),
	}

	tmplContent, err := loadTemplate("middleware.go.tmpl")
	if err != nil {
		return err
	}

	if err := generateFile(path, tmplContent, data); err != nil {
		return err
	}

	fmt.Printf("✓ Middleware created: %s\n", path)
	return nil
}

func toSnakeCase(s string) string {
	var result strings.Builder
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			result.WriteByte('_')
		}
		result.WriteRune(r)
	}
	return strings.ToLower(result.String())
}

// getTemplatesDir returns the path to the templates directory.
// It tries to find it relative to the current working directory first,
// then relative to the command file location.
func getTemplatesDir() (string, error) {
	// Try current working directory first
	cwd, err := os.Getwd()
	if err == nil {
		templatesPath := filepath.Join(cwd, "templates")
		if _, err := os.Stat(templatesPath); err == nil {
			return templatesPath, nil
		}
	}

	// Try relative to command file location
	_, filename, _, _ := runtime.Caller(0)
	cmdDir := filepath.Dir(filename)
	templatesPath := filepath.Join(cmdDir, "../../../templates")
	if _, err := os.Stat(templatesPath); err == nil {
		return templatesPath, nil
	}

	return "", fmt.Errorf("templates directory not found")
}

// loadTemplate loads a template file from the templates directory.
func loadTemplate(filename string) (string, error) {
	templatesDir, err := getTemplatesDir()
	if err != nil {
		return "", err
	}

	path := filepath.Join(templatesDir, filename)
	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read template %s: %w", filename, err)
	}

	return string(content), nil
}
