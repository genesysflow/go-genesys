package notifications_test

import (
	"testing"

	"github.com/genesysflow/go-genesys/mail"
	"github.com/genesysflow/go-genesys/notifications"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type swappedNotification struct{}

func (n *swappedNotification) Via(notifications.Notifiable) []string { return []string{"mail"} }
func (n *swappedNotification) ToMail(notifications.Notifiable) *mail.Message {
	return mail.NewMessage().Subject("hello").Text("hi")
}

// A test swaps the mailer for an array and expects what the application
// sends to land in it - including notifications, which would otherwise
// keep delivering through the mailer they were built with.
func TestManagerUsesTheSwappedMailer(t *testing.T) {
	original := mail.NewArrayMailer(mail.Config{FromAddress: "blog@example.com"})
	manager := notifications.New(notifications.WithMailer(original))

	replacement := mail.NewArrayMailer(mail.Config{FromAddress: "blog@example.com"})
	manager.SetMailer(replacement)

	require.NoError(t, manager.Send(notifications.Route("mail", "ada@example.com"), &swappedNotification{}))

	replacement.AssertSentTo(t, "ada@example.com")
	assert.Empty(t, original.Sent())
}

// A manager built without a mailer falls back to the application's
// default one, rather than refusing to send.
func TestManagerFallsBackToTheDefaultMailer(t *testing.T) {
	fallback := mail.NewArrayMailer(mail.Config{FromAddress: "blog@example.com"})
	mail.SetDefaultMailer(fallback)
	t.Cleanup(func() { mail.SetDefaultMailer(nil) })

	manager := notifications.New()

	require.NoError(t, manager.Send(notifications.Route("mail", "ada@example.com"), &swappedNotification{}))
	fallback.AssertSentTo(t, "ada@example.com")
}
