package mail_test

import (
	"strings"
	"testing"

	"github.com/genesysflow/go-genesys/mail"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMessageValidation(t *testing.T) {
	_, err := mail.NewMessage().Bytes()
	assert.ErrorContains(t, err, "no from address")

	_, err = mail.NewMessage().From("a@b.c").Bytes()
	assert.ErrorContains(t, err, "no recipients")

	_, err = mail.NewMessage().From("a@b.c").To("x@y.z").Bytes()
	assert.ErrorContains(t, err, "no body")
}

func TestPlainMessageRendering(t *testing.T) {
	raw, err := mail.NewMessage().
		From("sender@example.com", "The Sender").
		To("rcpt@example.com").
		Subject("Hello").
		Text("plain body").
		Bytes()
	require.NoError(t, err)

	msg := string(raw)
	assert.Contains(t, msg, "From: ")
	assert.Contains(t, msg, "sender@example.com")
	assert.Contains(t, msg, "To: rcpt@example.com")
	assert.Contains(t, msg, "Subject: Hello")
	assert.Contains(t, msg, "Content-Type: text/plain")
	// Body is base64-encoded.
	assert.Contains(t, msg, "cGxhaW4gYm9keQ==")
}

func TestMultipartAlternative(t *testing.T) {
	raw, err := mail.NewMessage().
		From("s@example.com").
		To("r@example.com").
		Subject("Both").
		Text("text version").
		HTML("<b>html version</b>").
		Bytes()
	require.NoError(t, err)

	msg := string(raw)
	assert.Contains(t, msg, "multipart/alternative")
	assert.Contains(t, msg, "text/plain")
	assert.Contains(t, msg, "text/html")
}

func TestAttachments(t *testing.T) {
	raw, err := mail.NewMessage().
		From("s@example.com").
		To("r@example.com").
		Subject("File").
		Text("see attached").
		Attach("data.csv", []byte("a,b\n1,2"), "text/csv").
		Bytes()
	require.NoError(t, err)

	msg := string(raw)
	assert.Contains(t, msg, "multipart/mixed")
	assert.Contains(t, msg, `attachment; filename="data.csv"`)
	assert.Contains(t, msg, "text/csv")
}

func TestBccNotInHeaders(t *testing.T) {
	message := mail.NewMessage().
		From("s@example.com").
		To("to@example.com").
		Bcc("hidden@example.com").
		Subject("Secret").
		Text("body")

	raw, err := message.Bytes()
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "hidden@example.com")
	assert.Contains(t, strings.Join(message.Recipients(), ","), "hidden@example.com")
}

func TestArrayMailerCapturesAndAppliesDefaults(t *testing.T) {
	mailer := mail.NewArrayMailer(mail.Config{FromAddress: "default@example.com", FromName: "App"})

	message := mail.NewMessage().To("r@example.com").Subject("Hi").Text("body")
	require.NoError(t, mailer.Send(message))

	sent := mailer.Sent()
	require.Len(t, sent, 1)
	assert.Equal(t, "default@example.com", sent[0].FromAddress())
	assert.Equal(t, "Hi", sent[0].GetSubject())
}
