package mail_test

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"github.com/genesysflow/go-genesys/mail"
	"github.com/genesysflow/go-genesys/view"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mailViews(t *testing.T) *view.Manager {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "emails"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "emails", "welcome.html"),
		[]byte(`<h1>Welcome, {{.name}}!</h1>`), 0o644))
	return view.NewManager(view.Config{Path: root})
}

func TestViewHTMLRendersTemplatedEmail(t *testing.T) {
	views := mailViews(t)
	mailer := mail.NewArrayMailer(mail.Config{FromAddress: "app@example.com"})

	message := mail.NewMessage().
		To("ada@example.com").
		Subject("Welcome!").
		ViewHTML(views, "emails.welcome", map[string]any{"name": "Ada"})
	require.NoError(t, mailer.Send(message))

	raw, err := mailer.Sent()[0].Bytes()
	require.NoError(t, err)
	// HTML bodies are base64-encoded in the MIME output.
	encoded := base64.StdEncoding.EncodeToString([]byte("<h1>Welcome, Ada!</h1>"))
	assert.Contains(t, string(raw), encoded)
	assert.Contains(t, string(raw), `Content-Type: text/html`)
}

func TestViewHTMLDefersRenderErrors(t *testing.T) {
	views := mailViews(t)
	mailer := mail.NewArrayMailer(mail.Config{FromAddress: "app@example.com"})

	message := mail.NewMessage().
		To("ada@example.com").
		Subject("Broken").
		ViewHTML(views, "emails.missing", nil)

	err := mailer.Send(message)
	require.Error(t, err, "the deferred render error surfaces at send time")
	assert.Contains(t, err.Error(), "emails.missing")
}
