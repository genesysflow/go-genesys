package commands

import (
	"fmt"
	"path/filepath"

	"github.com/genesysflow/go-genesys/console/cli"
	"github.com/genesysflow/go-genesys/contracts"
)

// authScaffoldFiles are the files make:auth writes, as
// template -> path relative to the application root.
var authScaffoldFiles = []struct {
	template string
	path     string
}{
	{"auth_controller.go.tmpl", "app/http/auth/controller.go"},
	{"auth_requests.go.tmpl", "app/http/auth/requests.go"},
	{"auth_routes.go.tmpl", "app/http/auth/routes.go"},
	{"auth_login.html.tmpl", "resources/views/auth/login.html"},
	{"auth_register.html.tmpl", "resources/views/auth/register.html"},
	{"auth_forgot_password.html.tmpl", "resources/views/auth/forgot_password.html"},
	{"auth_reset_password.html.tmpl", "resources/views/auth/reset_password.html"},
}

// MakeAuthCommand scaffolds the authentication flows - login, register,
// logout and password reset - as controllers, requests, routes and
// views, Laravel's Breeze-style starter.
//
// Everything it writes is ordinary application code: the generated
// controller is wired to a guard the application supplies, because the
// framework does not know your user model.
func MakeAuthCommand(app contracts.Application) *cli.Command {
	return &cli.Command{
		Name:        "make:auth",
		Description: "Scaffold the login, registration and password-reset flows",
		Handle: func(c *cli.Context) error {
			base := c.App().BasePath()

			// Nothing is written until every target is clear, so a
			// half-written scaffold never has to be untangled.
			for _, file := range authScaffoldFiles {
				if exists(filepath.Join(base, filepath.FromSlash(file.path))) {
					return fmt.Errorf("make:auth: %s already exists; move it aside first", file.path)
				}
			}

			for _, file := range authScaffoldFiles {
				content, err := render(file.template, map[string]string{})
				if err != nil {
					return err
				}

				path := filepath.Join(base, filepath.FromSlash(file.path))
				if err := writeGenerated(path, content); err != nil {
					return fmt.Errorf("make:auth: %w", err)
				}
				c.Line("Created: " + file.path)
			}

			c.NewLine()
			c.Info("Authentication scaffolding written.")
			c.Line("Wire it up where you register routes:")
			c.NewLine()
			c.Line(`    controller := &auth.Controller{`)
			c.Line(`        Guard: sessionGuard,`)
			c.Line(`        Users: userProvider,`)
			c.Line(`        Create: func(email, hashed string) (genesysauth.Authenticatable, error) { ... },`)
			c.Line(`    }`)
			c.Line(`    controller.Routes(router)`)

			return nil
		},
	}
}

// exists reports whether a path is already taken.
func exists(path string) bool {
	_, err := osStat(path)
	return err == nil
}
