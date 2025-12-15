package commands

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/genesysflow/go-genesys/contracts"
	"github.com/genesysflow/go-genesys/support"
	"github.com/genesysflow/go-genesys/templates"
	"github.com/spf13/cobra"
)

// MakeMigrationCommand creates the make:migration command.
func MakeMigrationCommand(app contracts.Application) *cobra.Command {
	return &cobra.Command{
		Use:   "make:migration <name>",
		Short: "Create a new database migration",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return createMigration(app, args[0])
		},
	}
}

// MakeControllerCommand creates the make:controller command.
func MakeControllerCommand(app contracts.Application) *cobra.Command {
	var resource bool

	cmd := &cobra.Command{
		Use:   "make:controller <name>",
		Short: "Create a new controller",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return createController(app, args[0], resource)
		},
	}

	cmd.Flags().BoolVarP(&resource, "resource", "r", false, "Generate a resource controller")
	return cmd
}

// MakeModelCommand creates the make:model command.
func MakeModelCommand(app contracts.Application) *cobra.Command {
	return &cobra.Command{
		Use:   "make:model <name>",
		Short: "Create a new model",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return createModel(app, args[0])
		},
	}
}

// MakeMiddlewareCommand creates the make:middleware command.
func MakeMiddlewareCommand(app contracts.Application) *cobra.Command {
	return &cobra.Command{
		Use:   "make:middleware <name>",
		Short: "Create a new middleware",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return createMiddleware(app, args[0])
		},
	}
}

// MakeProviderCommand creates the make:provider command.
func MakeProviderCommand(app contracts.Application) *cobra.Command {
	return &cobra.Command{
		Use:   "make:provider <name>",
		Short: "Create a new service provider",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return createProvider(app, args[0])
		},
	}
}

// =============================================================================
// Implementation Functions
// =============================================================================

func createMigration(app contracts.Application, name string) error {
	basePath := app.BasePath()
	dir := filepath.Join(basePath, "database", "migrations")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	timestamp := time.Now().Format("2006_01_02_150405")
	migrationName := support.ToSnakeCase(name)
	filename := timestamp + "_" + migrationName + ".go"
	path := filepath.Join(dir, filename)

	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("migration already exists: %s", path)
	}

	data := map[string]string{
		"Name":      support.ToPascalCase(name),
		"Timestamp": timestamp,
		"LowerName": migrationName,
	}

	content, err := render("migration.go.tmpl", data)
	if err != nil {
		return err
	}

	if err := os.WriteFile(path, content, 0644); err != nil {
		return err
	}

	// change the app.Register(&providers.MigrationServiceProvider{ Migrations: []migrations.Migration{...} })
	// to include the new migration.
	// there is a comment line there to help locate the spot. // DO NOT DELETE: Add new migrations here
	bootstrapDir := filepath.Join(basePath, "bootstrap")
	txt, err := os.ReadFile(bootstrapDir + "/app.go")
	if err != nil {
		return fmt.Errorf("failed to read bootstrap/app.go: %w", err)
	}
	newTxt := strings.Replace(
		string(txt),
		"// DO NOT DELETE: Add new migrations here\n",
		"&m."+support.ToPascalCase(name)+"{},\n\t\t\t// DO NOT DELETE: Add new migrations here\n",
		1,
	)
	if err := os.WriteFile(bootstrapDir+"/app.go", []byte(newTxt), 0644); err != nil {
		return fmt.Errorf("failed to update bootstrap/app.go: %w", err)
	}

	fmt.Printf("✓ Migration created: %s\n", path)
	return nil
}

func createController(app contracts.Application, name string, resource bool) error {
	basePath := app.BasePath()
	controllerName := support.ToPascalCase(name)
	if !strings.HasSuffix(controllerName, "Controller") {
		controllerName += "Controller"
	}

	dir := filepath.Join(basePath, "app", "controllers")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	baseName := strings.TrimSuffix(controllerName, "Controller")
	filename := support.ToSnakeCase(baseName) + "_controller.go"
	path := filepath.Join(dir, filename)

	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("controller already exists: %s", path)
	}

	data := map[string]string{
		"Package":   "controllers",
		"Name":      baseName,
		"LowerName": strings.ToLower(baseName),
		"RouteName": strings.ToLower(baseName),
	}

	var content []byte
	var err error
	if resource {
		content, err = render("controller.go.tmpl", data)
	} else {
		content, err = render("controller_simple.go.tmpl", data)
	}
	if err != nil {
		return err
	}

	if err := os.WriteFile(path, content, 0644); err != nil {
		return err
	}

	fmt.Printf("✓ Controller created: %s\n", path)
	return nil
}

func createModel(app contracts.Application, name string) error {
	basePath := app.BasePath()
	modelName := support.ToPascalCase(name)

	dir := filepath.Join(basePath, "app", "models")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	filename := support.ToSnakeCase(modelName) + ".go"
	path := filepath.Join(dir, filename)

	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("model already exists: %s", path)
	}

	tableName := support.ToSnakeCase(modelName) + "s"
	data := map[string]string{
		"Name":      modelName,
		"LowerName": strings.ToLower(modelName),
		"TableName": tableName,
	}

	content, err := render("model.go.tmpl", data)
	if err != nil {
		return err
	}

	if err := os.WriteFile(path, content, 0644); err != nil {
		return err
	}

	fmt.Printf("✓ Model created: %s\n", path)
	return nil
}

func createMiddleware(app contracts.Application, name string) error {
	basePath := app.BasePath()
	middlewareName := support.ToPascalCase(name)
	middlewareName = strings.TrimSuffix(middlewareName, "Middleware")

	dir := filepath.Join(basePath, "app", "middleware")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	filename := support.ToSnakeCase(middlewareName) + ".go"
	path := filepath.Join(dir, filename)

	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("middleware already exists: %s", path)
	}

	data := map[string]string{
		"Package":   "middleware",
		"Name":      middlewareName,
		"LowerName": strings.ToLower(middlewareName),
	}

	content, err := render("middleware.go.tmpl", data)
	if err != nil {
		return err
	}

	if err := os.WriteFile(path, content, 0644); err != nil {
		return err
	}

	fmt.Printf("✓ Middleware created: %s\n", path)
	return nil
}

func createProvider(app contracts.Application, name string) error {
	basePath := app.BasePath()
	providerName := support.ToPascalCase(name)
	if !strings.HasSuffix(providerName, "ServiceProvider") {
		providerName += "ServiceProvider"
	}

	dir := filepath.Join(basePath, "app", "providers")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	baseName := strings.TrimSuffix(providerName, "ServiceProvider")
	filename := support.ToSnakeCase(baseName) + "_provider.go"
	path := filepath.Join(dir, filename)

	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("provider already exists: %s", path)
	}

	data := map[string]string{
		"Package":   "providers",
		"Name":      baseName,
		"LowerName": strings.ToLower(baseName),
	}

	content, err := render("provider.go.tmpl", data)
	if err != nil {
		return err
	}

	if err := os.WriteFile(path, content, 0644); err != nil {
		return err
	}

	fmt.Printf("✓ Provider created: %s\n", path)
	return nil
}

func render(templateName string, data any) ([]byte, error) {
	tmpl, err := template.ParseFS(templates.FS, templateName)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
