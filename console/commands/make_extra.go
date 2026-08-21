package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/genesysflow/go-genesys/contracts"
	"github.com/genesysflow/go-genesys/support"
	"github.com/spf13/cobra"
)

// generatorSpec describes a simple template-driven make:* generator.
type generatorSpec struct {
	use      string
	short    string
	template string
	dir      string
	suffix   string // appended to the type name when missing (e.g. "Seeder")
}

func makeGeneratorCommand(app contracts.Application, spec generatorSpec) *cobra.Command {
	return &cobra.Command{
		Use:   spec.use,
		Short: spec.short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := support.ToPascalCase(args[0])
			if spec.suffix != "" && !strings.HasSuffix(name, spec.suffix) {
				name += spec.suffix
			}

			dir := filepath.Join(app.BasePath(), filepath.FromSlash(spec.dir))
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return fmt.Errorf("failed to create directory: %w", err)
			}

			path := filepath.Join(dir, support.ToSnakeCase(name)+".go")
			if _, err := os.Stat(path); err == nil {
				return fmt.Errorf("file already exists: %s", path)
			}

			content, err := render(spec.template, map[string]string{
				"Name":      name,
				"LowerName": support.ToSnakeCase(name),
			})
			if err != nil {
				return err
			}
			if err := os.WriteFile(path, content, 0o644); err != nil {
				return err
			}
			fmt.Printf("✓ Created: %s\n", path)
			return nil
		},
	}
}

// MakeJobCommand creates the make:job command.
func MakeJobCommand(app contracts.Application) *cobra.Command {
	return makeGeneratorCommand(app, generatorSpec{
		use: "make:job <name>", short: "Create a new queueable job",
		template: "job.go.tmpl", dir: "app/jobs", suffix: "Job",
	})
}

// MakeEventCommand creates the make:event command.
func MakeEventCommand(app contracts.Application) *cobra.Command {
	return makeGeneratorCommand(app, generatorSpec{
		use: "make:event <name>", short: "Create a new event",
		template: "event.go.tmpl", dir: "app/events",
	})
}

// MakeListenerCommand creates the make:listener command.
func MakeListenerCommand(app contracts.Application) *cobra.Command {
	return makeGeneratorCommand(app, generatorSpec{
		use: "make:listener <name>", short: "Create a new event listener",
		template: "listener.go.tmpl", dir: "app/listeners",
	})
}

// MakeSeederCommand creates the make:seeder command.
func MakeSeederCommand(app contracts.Application) *cobra.Command {
	return makeGeneratorCommand(app, generatorSpec{
		use: "make:seeder <name>", short: "Create a new database seeder",
		template: "seeder.go.tmpl", dir: "database/seeders", suffix: "Seeder",
	})
}

// MakeRequestCommand creates the make:request command.
func MakeRequestCommand(app contracts.Application) *cobra.Command {
	return makeGeneratorCommand(app, generatorSpec{
		use: "make:request <name>", short: "Create a new form request",
		template: "request.go.tmpl", dir: "app/requests", suffix: "Request",
	})
}

// MakeCommandCommand creates the make:command command.
func MakeCommandCommand(app contracts.Application) *cobra.Command {
	return makeGeneratorCommand(app, generatorSpec{
		use: "make:command <name>", short: "Create a new console command",
		template: "command.go.tmpl", dir: "app/console",
	})
}

// MakePolicyCommand creates the make:policy command.
func MakePolicyCommand(app contracts.Application) *cobra.Command {
	return makeGeneratorCommand(app, generatorSpec{
		use: "make:policy <name>", short: "Create a new authorization policy",
		template: "policy.go.tmpl", dir: "app/policies",
	})
}
