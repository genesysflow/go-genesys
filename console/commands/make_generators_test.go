package commands_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/genesysflow/go-genesys/console/cli"
	"github.com/genesysflow/go-genesys/console/commands"
	"github.com/genesysflow/go-genesys/contracts"
	"github.com/genesysflow/go-genesys/foundation"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// generatorApp roots an application at a temp directory so generators
// write there.
func generatorApp(t *testing.T) contracts.Application {
	t.Helper()
	app := foundation.New(t.TempDir())
	require.NoError(t, app.Boot())
	return app
}

func TestMakeGenerators(t *testing.T) {
	cases := []struct {
		command  func(contracts.Application) *cli.Command
		argument string
		path     string
		contains []string
	}{
		{commands.MakeMailCommand, "OrderShipped", "app/mail/order_shipped_mail.go", []string{"package mail", "OrderShippedMail"}},
		{commands.MakeNotificationCommand, "InvoicePaid", "app/notifications/invoice_paid_notification.go", []string{"package notifications", "InvoicePaidNotification"}},
		{commands.MakeFactoryCommand, "User", "database/factories/user_factory.go", []string{"package factories", "UserFactory"}},
		{commands.MakeResourceCommand, "User", "app/resources/user_resource.go", []string{"package resources", "UserResource"}},
		{commands.MakeRuleCommand, "Uppercase", "app/rules/uppercase_rule.go", []string{"package rules", "UppercaseRule"}},
		{commands.MakeObserverCommand, "User", "app/observers/user_observer.go", []string{"package observers", "UserObserver"}},
		{commands.MakeCastCommand, "Money", "app/casts/money_cast.go", []string{"package casts", "MoneyCast"}},
		{commands.MakeScopeCommand, "Active", "app/scopes/active_scope.go", []string{"package scopes", "ActiveScope"}},
		{commands.MakeChannelCommand, "Order", "app/broadcasting/order_channel.go", []string{"package broadcasting", "OrderChannel"}},
		{commands.MakeExceptionCommand, "PaymentFailed", "app/exceptions/payment_failed_exception.go", []string{"package exceptions", "PaymentFailedException"}},
		{commands.MakeEnumCommand, "OrderStatus", "app/enums/order_status.go", []string{"package enums", "OrderStatus"}},
		{commands.MakePolicyCommand, "Post", "app/policies/post_policy.go", []string{"package policies", "PostPolicy"}},
	}

	for _, testCase := range cases {
		t.Run(testCase.path, func(t *testing.T) {
			app := generatorApp(t)

			out, err := runCommand(t, app, testCase.command(app), testCase.argument)
			require.NoError(t, err)
			assert.Contains(t, out, testCase.path)

			full := filepath.Join(app.BasePath(), filepath.FromSlash(testCase.path))
			content, err := os.ReadFile(full)
			require.NoError(t, err, "generated file should exist")

			for _, want := range testCase.contains {
				assert.Contains(t, string(content), want)
			}

			// A stub that does not parse is worse than no stub: the user
			// has to debug the generator instead of writing code.
			_, parseErr := parser.ParseFile(token.NewFileSet(), full, content, parser.AllErrors)
			assert.NoError(t, parseErr, "generated Go should parse")
		})
	}
}

// A generator must never overwrite work in progress.
func TestMakeGeneratorRefusesToOverwrite(t *testing.T) {
	app := generatorApp(t)

	_, err := runCommand(t, app, commands.MakeMailCommand(app), "OrderShipped")
	require.NoError(t, err)

	path := filepath.Join(app.BasePath(), "app", "mail", "order_shipped_mail.go")
	require.NoError(t, os.WriteFile(path, []byte("package mail // edited"), 0o600))

	_, err = runCommand(t, app, commands.MakeMailCommand(app), "OrderShipped")
	require.Error(t, err)

	content, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Contains(t, string(content), "edited", "the existing file must be left alone")
}

// make:test writes a Go test file, which has to end in _test.go to be
// compiled as one.
func TestMakeTestCommand(t *testing.T) {
	app := generatorApp(t)

	out, err := runCommand(t, app, commands.MakeTestCommand(app), "CreatesUser")
	require.NoError(t, err)
	assert.Contains(t, out, "_test.go")

	path := filepath.Join(app.BasePath(), "tests", "creates_user_test.go")
	content, err := os.ReadFile(path)
	require.NoError(t, err)

	assert.Contains(t, string(content), "func TestCreatesUser(t *testing.T)")
	_, parseErr := parser.ParseFile(token.NewFileSet(), path, content, parser.AllErrors)
	assert.NoError(t, parseErr)
}

// make:view writes an HTML template at the dot-notation name the view
// layer resolves.
func TestMakeViewCommand(t *testing.T) {
	app := generatorApp(t)

	out, err := runCommand(t, app, commands.MakeViewCommand(app), "users.index")
	require.NoError(t, err)
	assert.Contains(t, out, "users/index.html")

	content, err := os.ReadFile(filepath.Join(app.BasePath(), "resources", "views", "users", "index.html"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "users.index")
}

func TestMakeComponentCommand(t *testing.T) {
	app := generatorApp(t)

	_, err := runCommand(t, app, commands.MakeComponentCommand(app), "alert")
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(app.BasePath(), "resources", "views", "components", "alert.html"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "component")
}

// A view name may not escape the views directory.
func TestMakeViewRejectsTraversal(t *testing.T) {
	app := generatorApp(t)

	_, err := runCommand(t, app, commands.MakeViewCommand(app), "../../etc/passwd")
	require.Error(t, err)

	_, statErr := os.Stat(filepath.Join(filepath.Dir(app.BasePath()), "etc"))
	assert.True(t, os.IsNotExist(statErr), "nothing should be written outside the views directory")
}

// A stub that parses but does not compile still costs the user a
// debugging session, so every generated Go stub is built against the
// framework it references.
func TestGeneratedStubsCompile(t *testing.T) {
	if testing.Short() {
		t.Skip("compiling the generated stubs is slow")
	}

	generators := map[string]func(contracts.Application) *cli.Command{
		"mail":          commands.MakeMailCommand,
		"notifications": commands.MakeNotificationCommand,
		"factories":     commands.MakeFactoryCommand,
		"resources":     commands.MakeResourceCommand,
		"rules":         commands.MakeRuleCommand,
		"observers":     commands.MakeObserverCommand,
		"casts":         commands.MakeCastCommand,
		"scopes":        commands.MakeScopeCommand,
		"broadcasting":  commands.MakeChannelCommand,
		"exceptions":    commands.MakeExceptionCommand,
		"enums":         commands.MakeEnumCommand,
		"policies":      commands.MakePolicyCommand,
		"tests":         commands.MakeTestCommand,
	}

	// The stubs must be built from inside this module so their framework
	// imports resolve.
	root := repoRoot(t)
	workspace := filepath.Join(root, "console", "commands", "genstubs")
	require.NoError(t, os.MkdirAll(workspace, 0o755))
	t.Cleanup(func() { _ = os.RemoveAll(workspace) })

	for pkg, generator := range generators {
		app := generatorApp(t)
		_, err := runCommand(t, app, generator(app), "Sample")
		require.NoError(t, err)

		generated := findGoFile(t, app.BasePath())
		content, err := os.ReadFile(generated)
		require.NoError(t, err)

		target := filepath.Join(workspace, pkg)
		require.NoError(t, os.MkdirAll(target, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(target, filepath.Base(generated)), content, 0o600))
	}

	build := exec.Command("go", "build", "./console/commands/genstubs/...")
	build.Dir = root
	output, err := build.CombinedOutput()
	assert.NoError(t, err, "generated stubs should compile:\n%s", output)
}

// repoRoot walks up from the working directory to the module root.
func repoRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	require.NoError(t, err)

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		require.NotEqual(t, dir, parent, "go.mod not found above the working directory")
		dir = parent
	}
}

// findGoFile returns the single generated .go file under root.
func findGoFile(t *testing.T, root string) string {
	t.Helper()

	var found string
	require.NoError(t, filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && strings.HasSuffix(path, ".go") {
			found = path
		}
		return nil
	}))

	require.NotEmpty(t, found, "the generator should have written a .go file")
	return found
}
