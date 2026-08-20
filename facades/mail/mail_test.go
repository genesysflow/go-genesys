package mail_test

import (
	"testing"

	"github.com/genesysflow/go-genesys/facades/mail"
	basemail "github.com/genesysflow/go-genesys/mail"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFacadeSend(t *testing.T) {
	mailer := basemail.NewArrayMailer(basemail.Config{FromAddress: "app@example.com"})
	mail.SetInstance(mailer)
	t.Cleanup(func() { mail.SetInstance(nil) })

	message := mail.Message().To("user@example.com").Subject("Hello").Text("body")
	require.NoError(t, mail.Send(message))

	sent := mailer.Sent()
	require.Len(t, sent, 1)
	assert.Equal(t, "Hello", sent[0].GetSubject())
	assert.Equal(t, "app@example.com", sent[0].FromAddress())

	assert.NotNil(t, mail.GetInstance())
}

func TestFacadePanicsWithoutInstance(t *testing.T) {
	mail.SetInstance(nil)
	assert.Panics(t, func() { mail.Send(mail.Message()) })
}
