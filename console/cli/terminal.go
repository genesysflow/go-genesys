package cli

import (
	"bufio"
	"os"
	"strings"

	"golang.org/x/term"
)

// colorEnabled reports whether output is going to a terminal. Piped or
// captured output stays plain, so logs and test assertions are not full
// of escape codes. NO_COLOR is honoured (https://no-color.org).
var colorEnabled = isTerminal(os.Stdout) && os.Getenv("NO_COLOR") == ""

// isTerminal reports whether f is an interactive terminal.
func isTerminal(f *os.File) bool {
	if f == nil {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}

// readSecret reads one line with terminal echo disabled, so a password
// typed at a prompt is not left on screen or in a screenshot. When stdin
// is not a terminal (a pipe, a test) there is no echo to disable and the
// line is read normally.
func readSecret(reader *bufio.Reader) string {
	if isTerminal(os.Stdin) {
		raw, err := term.ReadPassword(int(os.Stdin.Fd()))
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(raw))
	}

	line, err := reader.ReadString('\n')
	if err != nil && line == "" {
		return ""
	}
	return strings.TrimSpace(line)
}
