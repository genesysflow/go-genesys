package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/genesysflow/go-genesys/console/cli"
	"github.com/genesysflow/go-genesys/contracts"
	"github.com/genesysflow/go-genesys/support"
)

// fileGenerator describes a make:* command that renders one stub.
type fileGenerator struct {
	name        string // command name, e.g. "make:mail"
	description string
	template    string
	dir         string // relative to the application root
	suffix      string // appended to the type name when missing
	fileSuffix  string // appended to the file name, e.g. "_test"
	ext         string // file extension, ".go" by default
}

// makeFileCommand builds the command for a generator spec.
func makeFileCommand(app contracts.Application, spec fileGenerator) *cli.Command {
	return &cli.Command{
		Name:        spec.name,
		Description: spec.description,
		Arguments: []cli.Argument{
			{Name: "name", Description: "The name of the generated type", Required: true},
		},
		Handle: func(c *cli.Context) error {
			name := support.ToPascalCase(c.Argument("name"))
			if name == "" {
				return fmt.Errorf("%s: a name is required", spec.name)
			}
			if spec.suffix != "" && !strings.HasSuffix(name, spec.suffix) {
				name += spec.suffix
			}

			ext := spec.ext
			if ext == "" {
				ext = ".go"
			}

			dir := filepath.Join(c.App().BasePath(), filepath.FromSlash(spec.dir))
			file := support.ToSnakeCase(name) + spec.fileSuffix + ext
			path := filepath.Join(dir, file)

			content, err := render(spec.template, map[string]string{
				"Name":      name,
				"LowerName": support.ToSnakeCase(name),
			})
			if err != nil {
				return err
			}

			if err := writeGenerated(path, content); err != nil {
				return fmt.Errorf("%s: %w", spec.name, err)
			}

			c.Info("Created: " + relativeToBase(c.App().BasePath(), path))
			return nil
		},
	}
}

// writeGenerated creates the parent directory and writes content, never
// replacing an existing file: a generator that overwrites is a generator
// that eats work in progress.
func writeGenerated(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("creating %s: %w", filepath.Dir(path), err)
	}
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("file already exists: %s", path)
	}
	return os.WriteFile(path, content, 0o600)
}

// relativeToBase renders a path for display, relative to the app root.
func relativeToBase(base, path string) string {
	if rel, err := filepath.Rel(base, path); err == nil {
		return filepath.ToSlash(rel)
	}
	return path
}

// MakeMailCommand creates the make:mail command.
func MakeMailCommand(app contracts.Application) *cli.Command {
	return makeFileCommand(app, fileGenerator{
		name: "make:mail", description: "Create a new mailable",
		template: "mail.go.tmpl", dir: "app/mail", suffix: "Mail",
	})
}

// MakeNotificationCommand creates the make:notification command.
func MakeNotificationCommand(app contracts.Application) *cli.Command {
	return makeFileCommand(app, fileGenerator{
		name: "make:notification", description: "Create a new notification",
		template: "notification.go.tmpl", dir: "app/notifications", suffix: "Notification",
	})
}

// MakeFactoryCommand creates the make:factory command.
func MakeFactoryCommand(app contracts.Application) *cli.Command {
	return makeFileCommand(app, fileGenerator{
		name: "make:factory", description: "Create a new model factory",
		template: "factory.go.tmpl", dir: "database/factories", suffix: "Factory",
	})
}

// MakeResourceCommand creates the make:resource command.
func MakeResourceCommand(app contracts.Application) *cli.Command {
	return makeFileCommand(app, fileGenerator{
		name: "make:resource", description: "Create a new API resource",
		template: "resource.go.tmpl", dir: "app/resources", suffix: "Resource",
	})
}

// MakeRuleCommand creates the make:rule command.
func MakeRuleCommand(app contracts.Application) *cli.Command {
	return makeFileCommand(app, fileGenerator{
		name: "make:rule", description: "Create a new validation rule",
		template: "rule.go.tmpl", dir: "app/rules", suffix: "Rule",
	})
}

// MakeObserverCommand creates the make:observer command.
func MakeObserverCommand(app contracts.Application) *cli.Command {
	return makeFileCommand(app, fileGenerator{
		name: "make:observer", description: "Create a new model observer",
		template: "observer.go.tmpl", dir: "app/observers", suffix: "Observer",
	})
}

// MakeCastCommand creates the make:cast command.
func MakeCastCommand(app contracts.Application) *cli.Command {
	return makeFileCommand(app, fileGenerator{
		name: "make:cast", description: "Create a new attribute cast",
		template: "cast.go.tmpl", dir: "app/casts", suffix: "Cast",
	})
}

// MakeScopeCommand creates the make:scope command.
func MakeScopeCommand(app contracts.Application) *cli.Command {
	return makeFileCommand(app, fileGenerator{
		name: "make:scope", description: "Create a new query scope",
		template: "scope.go.tmpl", dir: "app/scopes", suffix: "Scope",
	})
}

// MakeChannelCommand creates the make:channel command.
func MakeChannelCommand(app contracts.Application) *cli.Command {
	return makeFileCommand(app, fileGenerator{
		name: "make:channel", description: "Create a new broadcast channel authorizer",
		template: "channel.go.tmpl", dir: "app/broadcasting", suffix: "Channel",
	})
}

// MakeExceptionCommand creates the make:exception command.
func MakeExceptionCommand(app contracts.Application) *cli.Command {
	return makeFileCommand(app, fileGenerator{
		name: "make:exception", description: "Create a new application error type",
		template: "exception.go.tmpl", dir: "app/exceptions", suffix: "Exception",
	})
}

// MakeEnumCommand creates the make:enum command.
func MakeEnumCommand(app contracts.Application) *cli.Command {
	return makeFileCommand(app, fileGenerator{
		name: "make:enum", description: "Create a new enum type",
		template: "enum.go.tmpl", dir: "app/enums",
	})
}

// MakeTestCommand creates the make:test command. The file name carries
// the _test suffix Go requires to compile it as a test.
func MakeTestCommand(app contracts.Application) *cli.Command {
	return makeFileCommand(app, fileGenerator{
		name: "make:test", description: "Create a new test",
		template: "test.go.tmpl", dir: "tests", fileSuffix: "_test",
	})
}

// MakeViewCommand creates the make:view command, writing a template at
// the dot-notation name the view layer resolves ("users.index").
func MakeViewCommand(app contracts.Application) *cli.Command {
	return makeViewFileCommand(app, "make:view", "Create a new view", "view.html.tmpl", "resources/views")
}

// MakeComponentCommand creates the make:component command.
func MakeComponentCommand(app contracts.Application) *cli.Command {
	return makeViewFileCommand(app, "make:component", "Create a new view component", "component.html.tmpl", "resources/views/components")
}

// makeViewFileCommand builds a generator that writes an HTML template
// under a views directory, addressed in dot notation.
func makeViewFileCommand(app contracts.Application, name, description, templateName, dir string) *cli.Command {
	return &cli.Command{
		Name:        name,
		Description: description,
		Arguments: []cli.Argument{
			{Name: "name", Description: `The view name in dot notation, e.g. "users.index"`, Required: true},
		},
		Handle: func(c *cli.Context) error {
			viewName := c.Argument("name")

			relative, err := viewPath(viewName)
			if err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}

			content, err := render(templateName, map[string]string{
				"Name":      viewName,
				"LowerName": strings.ToLower(viewName),
			})
			if err != nil {
				return err
			}

			path := filepath.Join(c.App().BasePath(), filepath.FromSlash(dir), relative)
			if err := writeGenerated(path, content); err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}

			c.Info("Created: " + relativeToBase(c.App().BasePath(), path))
			return nil
		},
	}
}

// viewPath turns "users.index" into "users/index.html", refusing any
// name that would escape the views directory.
func viewPath(name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("a view name is required")
	}

	segments := strings.Split(name, ".")
	for _, segment := range segments {
		if segment == "" || segment == "." || segment == ".." ||
			strings.ContainsAny(segment, `/\`) {
			return "", fmt.Errorf("invalid view name %q", name)
		}
	}

	return filepath.Join(append(segments[:len(segments)-1:len(segments)-1], segments[len(segments)-1]+".html")...), nil
}
