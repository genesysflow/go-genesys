package mail_test

import (
	"fmt"
	"testing"

	"github.com/genesysflow/go-genesys/mail"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mailT struct{ failures []string }

func (m *mailT) Helper() {}
func (m *mailT) Errorf(format string, args ...any) {
	m.failures = append(m.failures, fmt.Sprintf(format, args...))
}

func TestArrayMailerAssertions(t *testing.T) {
	mailer := mail.NewArrayMailer(mail.Config{FromAddress: "app@example.com"})

	require.NoError(t, mailer.Send(
		mail.NewMessage().To("ada@example.com").Subject("Welcome").Text("hi")))

	mailer.AssertSentCount(t, 1)
	mailer.AssertSentTo(t, "ada@example.com")
	mailer.AssertSent(t, func(m *mail.Message) bool {
		return m.GetSubject() == "Welcome"
	})
}

func TestArrayMailerAssertionFailures(t *testing.T) {
	mailer := mail.NewArrayMailer(mail.Config{FromAddress: "app@example.com"})
	rec := &mailT{}

	mailer.AssertSentCount(rec, 1)
	mailer.AssertSentTo(rec, "nobody@example.com")
	mailer.AssertSent(rec, func(m *mail.Message) bool { return true })

	require.NoError(t, mailer.Send(mail.NewMessage().To("x@y.z").Subject("s").Text("b")))
	mailer.AssertNothingSent(rec)

	assert.Len(t, rec.failures, 4)
}
