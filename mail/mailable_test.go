package mail_test

import (
	"fmt"
	"testing"

	"github.com/genesysflow/go-genesys/mail"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// orderShipped is a mailable in the shape an application writes one.
type orderShipped struct {
	Recipient string
	Order     string
}

func (m *orderShipped) Envelope() mail.Envelope {
	return mail.Envelope{
		To:      []string{m.Recipient},
		Subject: "Your order has shipped",
		From:    "shop@example.com",
	}
}

func (m *orderShipped) Content() mail.Content {
	return mail.Content{
		HTML: fmt.Sprintf("<p>Order %s is on its way.</p>", m.Order),
		Text: fmt.Sprintf("Order %s is on its way.", m.Order),
	}
}

func TestSendMailable(t *testing.T) {
	mailer := mail.NewArrayMailer()

	require.NoError(t, mail.Send(mailer, &orderShipped{Recipient: "ada@example.com", Order: "A-1"}))

	sent := mailer.Sent()
	require.Len(t, sent, 1)
	assert.Equal(t, []string{"ada@example.com"}, sent[0].GetTo())
	assert.Equal(t, "Your order has shipped", sent[0].GetSubject())
	assert.Contains(t, sent[0].GetHTML(), "A-1")
	assert.Contains(t, sent[0].GetText(), "A-1")
}

// Mail.To(...) addresses a mailable at send time, the way a handler has
// the recipient but the mailable does not.
type receipt struct{ Total string }

func (m *receipt) Envelope() mail.Envelope {
	return mail.Envelope{Subject: "Your receipt", From: "shop@example.com"}
}

func (m *receipt) Content() mail.Content {
	return mail.Content{HTML: "<p>Total: " + m.Total + "</p>"}
}

func TestToSendsToTheGivenAddresses(t *testing.T) {
	mailer := mail.NewArrayMailer()

	require.NoError(t, mail.To(mailer, "ada@example.com", "grace@example.com").
		Cc("audit@example.com").
		Send(&receipt{Total: "9.99"}))

	sent := mailer.Sent()
	require.Len(t, sent, 1)
	assert.Equal(t, []string{"ada@example.com", "grace@example.com"}, sent[0].GetTo())
	assert.Contains(t, sent[0].Recipients(), "audit@example.com")
}

// A mailable with no recipient at all cannot be sent.
func TestMailableWithoutRecipientFails(t *testing.T) {
	mailer := mail.NewArrayMailer()

	err := mail.Send(mailer, &receipt{Total: "1.00"})
	require.Error(t, err)
	assert.Empty(t, mailer.Sent())
}

// --- views -----------------------------------------------------------

type stubRenderer struct{ rendered map[string]string }

func (r stubRenderer) RenderString(name string, data map[string]any) (string, error) {
	if body, ok := r.rendered[name]; ok {
		return body + fmt.Sprint(data["name"]), nil
	}
	return "", fmt.Errorf("view %s not found", name)
}

type welcome struct{ Name string }

func (m *welcome) Envelope() mail.Envelope {
	return mail.Envelope{To: []string{"ada@example.com"}, Subject: "Welcome", From: "hi@example.com"}
}

func (m *welcome) Content() mail.Content {
	return mail.Content{
		View: "emails.welcome",
		Data: map[string]any{"name": m.Name},
	}
}

func TestMailableRendersItsView(t *testing.T) {
	mailer := mail.NewArrayMailer()
	renderer := stubRenderer{rendered: map[string]string{"emails.welcome": "Hello "}}

	require.NoError(t, mail.SendWith(mailer, renderer, &welcome{Name: "Ada"}))

	sent := mailer.Sent()
	require.Len(t, sent, 1)
	assert.Equal(t, "Hello Ada", sent[0].GetHTML())
}

// A view that cannot render must fail the send, not deliver a blank
// email.
func TestMailableViewErrorFailsTheSend(t *testing.T) {
	mailer := mail.NewArrayMailer()
	renderer := stubRenderer{rendered: map[string]string{}}

	err := mail.SendWith(mailer, renderer, &welcome{Name: "Ada"})
	require.Error(t, err)
	assert.Empty(t, mailer.Sent())
}

// A mailable naming a view with no renderer available is a wiring
// mistake, and must not send an empty message.
func TestMailableViewWithoutRendererFails(t *testing.T) {
	mailer := mail.NewArrayMailer()

	err := mail.Send(mailer, &welcome{Name: "Ada"})
	require.Error(t, err)
	assert.Empty(t, mailer.Sent())
}

// --- attachments -----------------------------------------------------

type invoice struct{}

func (m *invoice) Envelope() mail.Envelope {
	return mail.Envelope{To: []string{"ada@example.com"}, Subject: "Invoice", From: "billing@example.com"}
}

func (m *invoice) Content() mail.Content {
	return mail.Content{Text: "Your invoice is attached."}
}

func (m *invoice) Attachments() []mail.Attachment {
	return []mail.Attachment{
		{Filename: "invoice.pdf", Content: []byte("%PDF-1.4"), ContentType: "application/pdf"},
	}
}

func TestMailableAttachments(t *testing.T) {
	mailer := mail.NewArrayMailer()

	require.NoError(t, mail.Send(mailer, &invoice{}))

	raw, err := mailer.Sent()[0].Bytes()
	require.NoError(t, err)
	assert.Contains(t, string(raw), "invoice.pdf")
	assert.Contains(t, string(raw), "application/pdf")
}
