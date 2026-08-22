package cli

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/genesysflow/go-genesys/contracts"
	"github.com/spf13/cobra"
)

// Argument is a positional argument a command accepts.
type Argument struct {
	// Name is how the argument is read back: c.Argument("name").
	Name string

	// Description is shown in the command's help.
	Description string

	// Required fails the command when the argument is absent, rather than
	// letting it act on an empty value.
	Required bool

	// Default is used when an optional argument is absent.
	Default string
}

// Option is a flag a command accepts. An Option with no Default is a
// boolean flag (`--force`); one with a Default takes a value
// (`--queue=high`).
type Option struct {
	// Name is the long flag name, read back with c.Option(name).
	Name string

	// Shorthand is the optional single-letter form.
	Shorthand string

	// Description is shown in the command's help.
	Description string

	// Default is the value when the flag is absent. Leave empty for a
	// boolean flag, or set TakesValue for a value option with no
	// sensible default.
	Default string

	// TakesValue marks an option that takes a value even though its
	// default is empty - "--since 2020-01-01" rather than a switch.
	// Without it an option with no default is a boolean flag, which is
	// the common case (--force, --seed).
	TakesValue bool
}

// Command describes a console command in the shape Laravel's commands
// take: a name, a description, the arguments and options it accepts, and
// a handler that talks to the user through a Context.
//
//	var Greet = &console.Command{
//	    Name:        "greet",
//	    Description: "Greet someone",
//	    Arguments:   []console.Argument{{Name: "name", Required: true}},
//	    Options:     []console.Option{{Name: "shout"}},
//	    Handle: func(c *console.Context) error {
//	        c.Info("Hello, " + c.Argument("name"))
//	        return nil
//	    },
//	}
type Command struct {
	Name        string
	Description string
	Long        string
	Arguments   []Argument
	Options     []Option
	Handle      func(c *Context) error

	// Hidden keeps the command out of the listing.
	Hidden bool
}

// noInteractionFlag mirrors artisan's --no-interaction: prompts take
// their default instead of blocking on input that will never arrive in a
// cron job or a CI step.
const noInteractionFlag = "no-interaction"

// Cobra builds the cobra command that runs this command for app.
func (c *Command) Cobra(app contracts.Application) *cobra.Command {
	cmd := &cobra.Command{
		Use:    c.usage(),
		Short:  c.Description,
		Long:   c.Long,
		Hidden: c.Hidden,
		// The handler's error is the command's outcome; cobra printing it
		// again as a usage dump only buries it.
		SilenceUsage: true,
	}

	for _, option := range c.Options {
		if option.Default == "" && !option.TakesValue {
			cmd.Flags().BoolP(option.Name, option.Shorthand, false, option.Description)
			continue
		}
		cmd.Flags().StringP(option.Name, option.Shorthand, option.Default, option.Description)
	}
	cmd.Flags().Bool(noInteractionFlag, false, "Do not ask any interactive question")

	required := 0
	for _, argument := range c.Arguments {
		if argument.Required {
			required++
		}
	}
	cmd.Args = cobra.RangeArgs(required, len(c.Arguments))

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if c.Handle == nil {
			return fmt.Errorf("console: command %q has no handler", c.Name)
		}
		return c.Handle(newContext(app, c, cmd, args))
	}

	return cmd
}

// usage renders the cobra use line: "greet <name> [greeting]".
func (c *Command) usage() string {
	parts := []string{c.Name}
	for _, argument := range c.Arguments {
		if argument.Required {
			parts = append(parts, "<"+argument.Name+">")
			continue
		}
		parts = append(parts, "["+argument.Name+"]")
	}
	return strings.Join(parts, " ")
}

// Context is what a running command talks to the user through: its
// arguments and options, output helpers, and prompts.
type Context struct {
	app       contracts.Application
	command   *Command
	cobra     *cobra.Command
	positions []string
	reader    *bufio.Reader
}

func newContext(app contracts.Application, command *Command, cmd *cobra.Command, args []string) *Context {
	return &Context{
		app:       app,
		command:   command,
		cobra:     cmd,
		positions: args,
		reader:    bufio.NewReader(cmd.InOrStdin()),
	}
}

// App returns the application the command runs against.
func (c *Context) App() contracts.Application { return c.app }

// Cobra returns the underlying cobra command, for the rare case a
// command needs a flag type the Option shape does not cover.
func (c *Context) Cobra() *cobra.Command { return c.cobra }

// Argument returns a positional argument by name, falling back to its
// declared default.
func (c *Context) Argument(name string) string {
	for i, argument := range c.command.Arguments {
		if argument.Name != name {
			continue
		}
		if i < len(c.positions) && c.positions[i] != "" {
			return c.positions[i]
		}
		return argument.Default
	}
	return ""
}

// Arguments returns every positional argument by name.
func (c *Context) Arguments() map[string]string {
	values := make(map[string]string, len(c.command.Arguments))
	for _, argument := range c.command.Arguments {
		values[argument.Name] = c.Argument(argument.Name)
	}
	return values
}

// Option returns a string flag's value.
func (c *Context) Option(name string) string {
	value, err := c.cobra.Flags().GetString(name)
	if err != nil {
		return ""
	}
	return value
}

// BoolOption returns a boolean flag's value.
func (c *Context) BoolOption(name string) bool {
	value, err := c.cobra.Flags().GetBool(name)
	if err != nil {
		return false
	}
	return value
}

// IntOption returns a flag's value parsed as an integer, or 0.
func (c *Context) IntOption(name string) int {
	value, err := strconv.Atoi(c.Option(name))
	if err != nil {
		return 0
	}
	return value
}

// --- output ----------------------------------------------------------

func (c *Context) out() io.Writer { return c.cobra.OutOrStdout() }

// Line writes a plain line.
func (c *Context) Line(message string) {
	fmt.Fprintln(c.out(), message)
}

// Linef writes a formatted plain line.
func (c *Context) Linef(format string, args ...any) {
	c.Line(fmt.Sprintf(format, args...))
}

// NewLine writes count blank lines (one by default).
func (c *Context) NewLine(count ...int) {
	lines := 1
	if len(count) > 0 {
		lines = count[0]
	}
	for i := 0; i < lines; i++ {
		fmt.Fprintln(c.out())
	}
}

// Info writes a success/information line.
func (c *Context) Info(message string) { c.Line(colorize("32", message)) }

// Comment writes a secondary line.
func (c *Context) Comment(message string) { c.Line(colorize("36", message)) }

// Warn writes a warning line.
func (c *Context) Warn(message string) { c.Line(colorize("33", message)) }

// Error writes an error line to stderr.
func (c *Context) Error(message string) {
	fmt.Fprintln(c.cobra.ErrOrStderr(), colorize("31", message))
}

// colorize wraps text in an ANSI colour when the output is a terminal.
// Tests and pipes get plain text, so output stays diffable.
func colorize(code, message string) string {
	if !colorEnabled {
		return message
	}
	return "\x1b[" + code + "m" + message + "\x1b[0m"
}

// Table writes rows under headers, padded to a common column width.
func (c *Context) Table(headers []string, rows [][]string) {
	widths := make([]int, len(headers))
	for i, header := range headers {
		widths[i] = len(header)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i < len(widths) && len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}

	c.Line(padRow(headers, widths))

	separators := make([]string, len(headers))
	for i, width := range widths {
		separators[i] = strings.Repeat("-", width)
	}
	c.Line(padRow(separators, widths))

	for _, row := range rows {
		c.Line(padRow(row, widths))
	}
}

// padRow pads each cell to its column width and joins them.
func padRow(cells []string, widths []int) string {
	padded := make([]string, 0, len(cells))
	for i, cell := range cells {
		width := 0
		if i < len(widths) {
			width = widths[i]
		}
		if i == len(cells)-1 {
			padded = append(padded, cell)
			continue
		}
		padded = append(padded, cell+strings.Repeat(" ", max(0, width-len(cell))))
	}
	return strings.TrimRight(strings.Join(padded, "  "), " ")
}

// WithProgressBar runs fn, rendering progress as it calls advance. The
// bar is redrawn in place on a terminal and printed once per step
// otherwise, so a log file does not fill with control characters.
func (c *Context) WithProgressBar(total int, fn func(advance func()) error) error {
	done := 0
	render := func() {
		if colorEnabled {
			fmt.Fprintf(c.out(), "\r%s %d/%d", progressBar(done, total), done, total)
			return
		}
		fmt.Fprintf(c.out(), "%d/%d\n", done, total)
	}

	err := fn(func() {
		done++
		render()
	})

	if colorEnabled {
		fmt.Fprintln(c.out())
	}
	return err
}

// progressBar renders a 30-cell bar.
func progressBar(done, total int) string {
	const width = 30
	filled := 0
	if total > 0 {
		filled = done * width / total
	}
	return "[" + strings.Repeat("=", filled) + strings.Repeat(" ", width-filled) + "]"
}

// --- prompts ---------------------------------------------------------

// interactive reports whether prompts should read from input.
func (c *Context) interactive() bool {
	noInteraction, err := c.cobra.Flags().GetBool(noInteractionFlag)
	if err != nil {
		return true
	}
	return !noInteraction
}

// readLine reads one answer, reporting whether input was available.
func (c *Context) readLine() (string, bool) {
	line, err := c.reader.ReadString('\n')
	if err != nil && line == "" {
		return "", false
	}
	return strings.TrimSpace(line), true
}

// Ask prompts for a value, returning the default when the answer is
// empty or input is unavailable.
func (c *Context) Ask(question string, defaultValue ...string) string {
	fallback := ""
	if len(defaultValue) > 0 {
		fallback = defaultValue[0]
	}

	if !c.interactive() {
		return fallback
	}

	if fallback != "" {
		fmt.Fprintf(c.out(), "%s [%s] ", question, fallback)
	} else {
		fmt.Fprintf(c.out(), "%s ", question)
	}

	answer, ok := c.readLine()
	if !ok || answer == "" {
		return fallback
	}
	return answer
}

// Secret prompts for a value without echoing it. The terminal's echo is
// only disabled when stdin is a terminal; elsewhere it reads normally.
func (c *Context) Secret(question string) string {
	if !c.interactive() {
		return ""
	}

	fmt.Fprintf(c.out(), "%s ", question)
	answer := readSecret(c.reader)
	fmt.Fprintln(c.out())
	return answer
}

// Confirm asks a yes/no question. An empty answer, or a non-interactive
// run, takes the default - a command in a cron job must not block on a
// prompt no one will answer.
func (c *Context) Confirm(question string, defaultValue bool) bool {
	if !c.interactive() {
		return defaultValue
	}

	hint := "y/N"
	if defaultValue {
		hint = "Y/n"
	}
	fmt.Fprintf(c.out(), "%s (%s) ", question, hint)

	answer, ok := c.readLine()
	if !ok || answer == "" {
		return defaultValue
	}

	switch strings.ToLower(answer) {
	case "y", "yes":
		return true
	case "n", "no":
		return false
	default:
		return defaultValue
	}
}

// Choice asks the user to pick one of the choices, by number or by
// value. An empty answer takes the default.
func (c *Context) Choice(question string, choices []string, defaultValue ...string) string {
	fallback := ""
	if len(defaultValue) > 0 {
		fallback = defaultValue[0]
	}

	if !c.interactive() || len(choices) == 0 {
		return fallback
	}

	c.Line(question)
	for i, choice := range choices {
		c.Linef("  [%d] %s", i+1, choice)
	}
	if fallback != "" {
		fmt.Fprintf(c.out(), "> [%s] ", fallback)
	} else {
		fmt.Fprint(c.out(), "> ")
	}

	answer, ok := c.readLine()
	if !ok || answer == "" {
		return fallback
	}

	if index, err := strconv.Atoi(answer); err == nil {
		if index >= 1 && index <= len(choices) {
			return choices[index-1]
		}
		return fallback
	}

	for _, choice := range choices {
		if strings.EqualFold(choice, answer) {
			return choice
		}
	}
	return fallback
}

// Call runs another registered command, Laravel's $this->call(). Output
// goes to this command's streams.
func (c *Context) Call(name string, args ...string) error {
	root := c.cobra.Root()

	if target, _, err := root.Find([]string{name}); err != nil || target == root {
		return fmt.Errorf("console: command %q not found", name)
	}

	// The callee inherits this command's streams so its output lands
	// wherever the caller's does.
	root.SetOut(c.out())
	root.SetErr(c.cobra.ErrOrStderr())
	root.SetIn(c.cobra.InOrStdin())

	root.SetArgs(append([]string{name}, args...))
	defer root.SetArgs(nil)

	return root.Execute()
}
