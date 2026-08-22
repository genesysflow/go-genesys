package cli_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/genesysflow/go-genesys/console"
	"github.com/genesysflow/go-genesys/console/cli"
	"github.com/genesysflow/go-genesys/foundation"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// run builds the command and executes it with the given argv, capturing
// output.
func run(t *testing.T, cmd *cli.Command, in string, argv ...string) (string, error) {
	t.Helper()

	app := foundation.New()
	var out bytes.Buffer

	cobraCmd := cmd.Cobra(app)
	cobraCmd.SetIn(strings.NewReader(in))
	cobraCmd.SetOut(&out)
	cobraCmd.SetErr(&out)
	cobraCmd.SetArgs(argv)

	err := cobraCmd.Execute()
	return out.String(), err
}

func TestCommandArgumentsAndOptions(t *testing.T) {
	cmd := &cli.Command{
		Name:        "greet",
		Description: "Greet someone",
		Arguments: []cli.Argument{
			{Name: "name", Description: "Who to greet", Required: true},
			{Name: "greeting", Description: "The greeting", Default: "Hello"},
		},
		Options: []cli.Option{
			{Name: "shout", Description: "Upper-case the greeting"},
			{Name: "times", Description: "How many times", Default: "1"},
		},
		Handle: func(c *cli.Context) error {
			line := c.Argument("greeting") + ", " + c.Argument("name")
			if c.BoolOption("shout") {
				line = strings.ToUpper(line)
			}
			for i := 0; i < c.IntOption("times"); i++ {
				c.Line(line)
			}
			return nil
		},
	}

	out, err := run(t, cmd, "", "Ada")
	require.NoError(t, err)
	assert.Equal(t, "Hello, Ada\n", out)

	out, err = run(t, cmd, "", "Ada", "Hi", "--shout", "--times", "2")
	require.NoError(t, err)
	assert.Equal(t, "HI, ADA\nHI, ADA\n", out)
}

// A required argument that is missing is an error, not a zero value the
// command silently acts on.
func TestCommandRequiredArgument(t *testing.T) {
	cmd := &cli.Command{
		Name:      "greet",
		Arguments: []cli.Argument{{Name: "name", Required: true}},
		Handle: func(c *cli.Context) error {
			c.Line(c.Argument("name"))
			return nil
		},
	}

	_, err := run(t, cmd, "")
	require.Error(t, err)
}

func TestCommandOutputHelpers(t *testing.T) {
	cmd := &cli.Command{
		Name: "report",
		Handle: func(c *cli.Context) error {
			c.Info("all good")
			c.Comment("a note")
			c.Warn("careful")
			c.Error("broken")
			c.NewLine()
			return nil
		},
	}

	out, err := run(t, cmd, "")
	require.NoError(t, err)

	for _, want := range []string{"all good", "a note", "careful", "broken"} {
		assert.Contains(t, out, want)
	}
}

func TestCommandTable(t *testing.T) {
	cmd := &cli.Command{
		Name: "listing",
		Handle: func(c *cli.Context) error {
			c.Table(
				[]string{"Name", "Driver"},
				[][]string{{"default", "postgres"}, {"cache", "redis"}},
			)
			return nil
		},
	}

	out, err := run(t, cmd, "")
	require.NoError(t, err)

	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	require.GreaterOrEqual(t, len(lines), 4)
	assert.Contains(t, lines[0], "Name")
	assert.Contains(t, lines[0], "Driver")
	// Columns are padded to a common width, so values line up.
	assert.Contains(t, out, "default")
	assert.Contains(t, out, "postgres")
	assert.Equal(t, strings.Index(lines[0], "Driver"), strings.Index(lines[2], "postgres"))
}

func TestCommandAsk(t *testing.T) {
	cmd := &cli.Command{
		Name: "setup",
		Handle: func(c *cli.Context) error {
			c.Line("name=" + c.Ask("Your name?"))
			c.Line("host=" + c.Ask("Host?", "localhost"))
			return nil
		},
	}

	out, err := run(t, cmd, "Ada\n\n")
	require.NoError(t, err)
	assert.Contains(t, out, "name=Ada")
	assert.Contains(t, out, "host=localhost")
}

func TestCommandConfirm(t *testing.T) {
	cmd := &cli.Command{
		Name: "danger",
		Handle: func(c *cli.Context) error {
			if c.Confirm("Really?", false) {
				c.Line("confirmed")
				return nil
			}
			c.Line("aborted")
			return nil
		},
	}

	out, err := run(t, cmd, "yes\n")
	require.NoError(t, err)
	assert.Contains(t, out, "confirmed")

	out, err = run(t, cmd, "\n")
	require.NoError(t, err)
	assert.Contains(t, out, "aborted", "an empty answer takes the default")

	out, err = run(t, cmd, "n\n")
	require.NoError(t, err)
	assert.Contains(t, out, "aborted")
}

// A destructive command in a non-interactive shell must not hang waiting
// for input that will never come; --no-interaction takes the default.
func TestCommandConfirmNonInteractive(t *testing.T) {
	cmd := &cli.Command{
		Name: "danger",
		Handle: func(c *cli.Context) error {
			if c.Confirm("Really?", true) {
				c.Line("confirmed")
			} else {
				c.Line("aborted")
			}
			return nil
		},
	}

	out, err := run(t, cmd, "", "--no-interaction")
	require.NoError(t, err)
	assert.Contains(t, out, "confirmed")
}

func TestCommandChoice(t *testing.T) {
	cmd := &cli.Command{
		Name: "pick",
		Handle: func(c *cli.Context) error {
			c.Line("picked=" + c.Choice("Driver?", []string{"postgres", "mysql", "sqlite"}, "sqlite"))
			return nil
		},
	}

	out, err := run(t, cmd, "2\n")
	require.NoError(t, err)
	assert.Contains(t, out, "picked=mysql")

	out, err = run(t, cmd, "mysql\n")
	require.NoError(t, err)
	assert.Contains(t, out, "picked=mysql")

	out, err = run(t, cmd, "\n")
	require.NoError(t, err)
	assert.Contains(t, out, "picked=sqlite")
}

func TestCommandProgressBar(t *testing.T) {
	cmd := &cli.Command{
		Name: "import",
		Handle: func(c *cli.Context) error {
			items := []string{"a", "b", "c"}
			return c.WithProgressBar(len(items), func(advance func()) error {
				for range items {
					advance()
				}
				return nil
			})
		},
	}

	out, err := run(t, cmd, "")
	require.NoError(t, err)
	assert.Contains(t, out, "3/3")
}

// Commands compose: one command can invoke another by name.
func TestCommandCall(t *testing.T) {
	app := foundation.New()
	kernel := console.NewKernel(app)

	inner := &cli.Command{
		Name:      "inner",
		Arguments: []cli.Argument{{Name: "value", Required: true}},
		Handle: func(c *cli.Context) error {
			c.Line("inner got " + c.Argument("value"))
			return nil
		},
	}
	outer := &cli.Command{
		Name: "outer",
		Handle: func(c *cli.Context) error {
			return c.Call("inner", "from-outer")
		},
	}

	kernel.AddCommand(inner.Cobra(app), outer.Cobra(app))

	var out bytes.Buffer
	root := kernel.RootCommand()
	root.SetOut(&out)
	root.SetErr(&out)

	require.NoError(t, kernel.Handle([]string{"outer"}))
	assert.Contains(t, out.String(), "inner got from-outer")
}

// The command reaches the application it was built for.
func TestCommandApp(t *testing.T) {
	cmd := &cli.Command{
		Name: "env",
		Handle: func(c *cli.Context) error {
			require.NotNil(t, c.App())
			c.Line(c.App().Environment())
			return nil
		},
	}

	out, err := run(t, cmd, "")
	require.NoError(t, err)
	assert.NotEmpty(t, strings.TrimSpace(out))
}

// An error from Handle reaches the caller rather than being printed and
// swallowed - the process exit code depends on it.
func TestCommandHandleErrorPropagates(t *testing.T) {
	cmd := &cli.Command{
		Name: "boom",
		Handle: func(c *cli.Context) error {
			return assert.AnError
		},
	}

	_, err := run(t, cmd, "")
	assert.ErrorIs(t, err, assert.AnError)
}

// An option that takes a value but has no sensible default is still an
// option that takes a value: without this, --since 2020-01-01 parses the
// date as a positional argument.
func TestStringOptionWithAnEmptyDefault(t *testing.T) {
	var got string

	command := &cli.Command{
		Name: "report",
		Options: []cli.Option{
			{Name: "since", Description: "Start date", TakesValue: true},
		},
		Handle: func(c *cli.Context) error {
			got = c.Option("since")
			return nil
		},
	}

	_, err := run(t, command, "", "--since", "2020-01-01")
	require.NoError(t, err)
	assert.Equal(t, "2020-01-01", got)

	// Absent, it is empty rather than the string "false".
	got = "unset"
	_, err = run(t, command, "")
	require.NoError(t, err)
	assert.Equal(t, "", got)
}

// An option with no default and no value is still a boolean switch, so
// existing commands keep working.
func TestOptionWithoutADefaultIsBoolean(t *testing.T) {
	var forced bool

	command := &cli.Command{
		Name:    "prune",
		Options: []cli.Option{{Name: "force"}},
		Handle: func(c *cli.Context) error {
			forced = c.BoolOption("force")
			return nil
		},
	}

	_, err := run(t, command, "", "--force")
	require.NoError(t, err)
	assert.True(t, forced)
}
