package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"text/template"

	"github.com/genesysflow/go-genesys/foundation"
	"github.com/spf13/cobra"
)

// TemplateData holds data for template rendering.
type TemplateData struct {
	Name       string
	Package    string
	ModulePath string
	LowerName  string
	RouteName  string
	TableName  string
}

// NewCmd creates the 'new' command.
func NewCmd() *cobra.Command {
	var moduleName string

	cmd := &cobra.Command{
		Use:   "new <project-name>",
		Short: "Create a new Go-Genesys project",
		Long: `Create a new Go-Genesys project with the recommended directory structure
and basic configuration files.

Example:
  genesys new myapp
  genesys new myapp --module github.com/username/myapp`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projectName := args[0]

			if moduleName == "" {
				moduleName = projectName
			}

			return createProject(projectName, moduleName)
		},
	}

	cmd.Flags().StringVarP(&moduleName, "module", "m", "", "Go module name (default: project name)")

	return cmd
}

func createProject(name, moduleName string) error {
	fmt.Printf("Creating new Go-Genesys project: %s\n", name)

	// Create project directory
	if err := os.MkdirAll(name, 0755); err != nil {
		return fmt.Errorf("failed to create project directory: %w", err)
	}

	// Create directory structure
	dirs := []string{
		"app/controllers",
		"app/middleware",
		"app/providers",
		"database/migrations",
		"bootstrap",
		"config",
		"routes",
		"storage/logs",
		"storage/cache",
		"storage/sessions",
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(filepath.Join(name, dir), 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	// Create .gitkeep files in storage directories
	gitkeepDirs := []string{
		"storage/logs",
		"storage/cache",
		"storage/sessions",
	}
	for _, dir := range gitkeepDirs {
		gitkeepPath := filepath.Join(name, dir, ".gitkeep")
		if err := os.WriteFile(gitkeepPath, []byte{}, 0644); err != nil {
			return fmt.Errorf("failed to create .gitkeep in %s: %w", dir, err)
		}
	}

	data := TemplateData{
		Name:       toPascalCase(name),
		Package:    moduleName,
		ModulePath: moduleName,
		LowerName:  strings.ToLower(name),
	}

	// Generate files from templates
	templates := map[string]string{
		"main.go":                               "main.go.tmpl",
		"bootstrap/app.go":                      "bootstrap_app.go.tmpl",
		"app/providers/app_service_provider.go": "app_service_provider.go.tmpl",
		"database/migrations/migrations.go":     "migrations.go.tmpl",
		"routes/routes.go":                      "routes.go.tmpl",
		"routes/web.go":                         "routes_web.go.tmpl",
		"routes/api.go":                         "routes_api.go.tmpl",
		".env":                                  "env.tmpl",
		".env.example":                          "env.tmpl",
		".gitignore":                            "gitignore.tmpl",
		"README.md":                             "readme.md.tmpl",
		"config/app.yaml":                       "config_app.yaml.tmpl",
		"config/logging.yaml":                   "config_logging.yaml.tmpl",
		"config/session.yaml":                   "config_session.yaml.tmpl",
		"config/database.yaml":                  "config_database.yaml.tmpl",
	}

	for filename, tmplFilename := range templates {
		tmplContent, err := loadTemplate(tmplFilename)
		if err != nil {
			return fmt.Errorf("failed to load template %s: %w", tmplFilename, err)
		}
		if err := generateFile(filepath.Join(name, filename), tmplContent, data); err != nil {
			return fmt.Errorf("failed to create %s: %w", filename, err)
		}
	}

	// Create go.mod
	goModContent := fmt.Sprintf(`module %s

go 1.22

require (
	github.com/genesysflow/go-genesys v%s
	github.com/spf13/cobra v1.8.1
)
`, moduleName, foundation.Version)

	if err := os.WriteFile(filepath.Join(name, "go.mod"), []byte(goModContent), 0644); err != nil {
		return fmt.Errorf("failed to create go.mod: %w", err)
	}

	fmt.Printf("\n✓ Project created successfully!\n\n")
	fmt.Printf("Next steps:\n")
	fmt.Printf("  cd %s\n", name)
	fmt.Printf("  go mod tidy\n")
	fmt.Printf("  go run main.go\n\n")

	return nil
}

func generateFile(path, tmplContent string, data TemplateData) error {
	tmpl, err := template.New("file").Parse(tmplContent)
	if err != nil {
		return err
	}

	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	return tmpl.Execute(file, data)
}

// Utility Functions
// =============================================================================

func toPascalCase(s string) string {
	words := strings.FieldsFunc(s, func(r rune) bool {
		return r == '_' || r == '-' || r == ' '
	})

	for i, word := range words {
		if len(word) > 0 {
			words[i] = strings.ToUpper(string(word[0])) + strings.ToLower(word[1:])
		}
	}

	return strings.Join(words, "")
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
