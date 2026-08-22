// Package mail holds the example application's mailables.
package mail

import (
	"fmt"

	genesysmail "github.com/genesysflow/go-genesys/mail"
)

// Welcome greets a new author. It is a mailable: the message is an
// object, so a handler can hand it to the mailer without building
// headers by hand.
type Welcome struct {
	Name  string
	Email string
}

// Envelope says who it is from and to, and what it is about.
func (m *Welcome) Envelope() genesysmail.Envelope {
	return genesysmail.Envelope{
		From:    "hello@example.com",
		Name:    "The Genesys Blog",
		To:      []string{m.Email},
		Subject: "Welcome to the blog, " + m.Name,
	}
}

// Content is the body.
func (m *Welcome) Content() genesysmail.Content {
	return genesysmail.Content{
		HTML: fmt.Sprintf(`<p>Welcome, %s. Your first draft is waiting.</p>`, m.Name),
		Text: fmt.Sprintf("Welcome, %s. Your first draft is waiting.", m.Name),
	}
}

// ResetLink carries a password-reset token.
type ResetLink struct {
	Email string
	URL   string
}

// Envelope addresses the reset mail.
func (m *ResetLink) Envelope() genesysmail.Envelope {
	return genesysmail.Envelope{
		From:    "hello@example.com",
		To:      []string{m.Email},
		Subject: "Reset your password",
	}
}

// Content links to the reset form.
func (m *ResetLink) Content() genesysmail.Content {
	return genesysmail.Content{
		HTML: fmt.Sprintf(`<p><a href="%s">Choose a new password</a>. The link expires in an hour.</p>`, m.URL),
		Text: "Choose a new password: " + m.URL,
	}
}
