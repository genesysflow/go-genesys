package mail

import "fmt"

// Envelope describes who a mailable is from and to, and what it is
// about, Laravel's Mailable envelope.
type Envelope struct {
	From    string
	Name    string // display name for From
	To      []string
	Cc      []string
	Bcc     []string
	ReplyTo string
	Subject string
	Headers map[string]string
}

// Content is a mailable's body. Set View to render through the view
// layer, or HTML/Text to supply the body directly. A View wins over
// HTML when both are set.
type Content struct {
	View     string
	TextView string
	Data     map[string]any
	HTML     string
	Text     string
}

// Attachment is a file sent with a mailable.
type Attachment struct {
	Filename    string
	Content     []byte
	ContentType string
}

// Mailable is an email as an object rather than a hand-built message:
//
//	type OrderShipped struct{ Order *models.Order }
//
//	func (m *OrderShipped) Envelope() mail.Envelope {
//	    return mail.Envelope{To: []string{m.Order.Email}, Subject: "Your order has shipped"}
//	}
//
//	func (m *OrderShipped) Content() mail.Content {
//	    return mail.Content{View: "emails.shipped", Data: map[string]any{"order": m.Order}}
//	}
//
//	mail.To(mailer, user.Email).Send(&OrderShipped{Order: order})
type Mailable interface {
	Envelope() Envelope
	Content() Content
}

// HasAttachments is implemented by mailables that send files.
type HasAttachments interface {
	Attachments() []Attachment
}

// Message builds the message a mailable describes. renderer may be nil
// when the mailable supplies its body directly; a mailable naming a view
// with no renderer is a wiring mistake and reports one rather than
// sending a blank email.
func Build(mailable Mailable, renderer Renderer) (*Message, error) {
	envelope := mailable.Envelope()
	content := mailable.Content()

	message := NewMessage()
	if envelope.From != "" {
		message.From(envelope.From, envelope.Name)
	}
	message.To(envelope.To...)
	message.Cc(envelope.Cc...)
	message.Bcc(envelope.Bcc...)
	if envelope.ReplyTo != "" {
		message.ReplyTo(envelope.ReplyTo)
	}
	message.Subject(envelope.Subject)
	for key, value := range envelope.Headers {
		message.Header(key, value)
	}

	if content.HTML != "" {
		message.HTML(content.HTML)
	}
	if content.Text != "" {
		message.Text(content.Text)
	}

	if content.View != "" {
		if renderer == nil {
			return nil, fmt.Errorf("mail: %T names the view %q but no renderer is available", mailable, content.View)
		}
		html, err := renderer.RenderString(content.View, content.Data)
		if err != nil {
			return nil, fmt.Errorf("mail: rendering %q: %w", content.View, err)
		}
		message.HTML(html)
	}
	if content.TextView != "" {
		if renderer == nil {
			return nil, fmt.Errorf("mail: %T names the view %q but no renderer is available", mailable, content.TextView)
		}
		text, err := renderer.RenderString(content.TextView, content.Data)
		if err != nil {
			return nil, fmt.Errorf("mail: rendering %q: %w", content.TextView, err)
		}
		message.Text(text)
	}

	if attacher, ok := mailable.(HasAttachments); ok {
		for _, attachment := range attacher.Attachments() {
			message.Attach(attachment.Filename, attachment.Content, attachment.ContentType)
		}
	}

	return message, nil
}

// Send builds and sends a mailable that supplies its own body.
func Send(mailer Mailer, mailable Mailable) error {
	return SendWith(mailer, nil, mailable)
}

// SendWith builds and sends a mailable, rendering its views through the
// given renderer.
func SendWith(mailer Mailer, renderer Renderer, mailable Mailable) error {
	message, err := Build(mailable, renderer)
	if err != nil {
		return err
	}
	return mailer.Send(message)
}

// SendDefault builds and sends a mailable through the application's
// default mailer, resolved when the message is sent rather than when the
// handler was wired.
//
// That is what a handler usually wants: it has a mailable and no mailer
// to hand, and capturing one at wiring time is what stops a test's array
// mailer from ever seeing the message.
func SendDefault(mailable Mailable) error {
	mailer := DefaultMailer()
	if mailer == nil {
		return fmt.Errorf("mail: no default mailer installed - register the MailServiceProvider")
	}
	return Send(mailer, mailable)
}

// Pending addresses a mailable at send time, Laravel's
// `Mail::to($user)->send(...)`: the handler knows the recipient even
// when the mailable does not.
type Pending struct {
	mailer   Mailer
	renderer Renderer
	to       []string
	cc       []string
	bcc      []string
}

// To starts a pending send to the given addresses.
func To(mailer Mailer, addresses ...string) *Pending {
	return &Pending{mailer: mailer, to: addresses}
}

// Cc adds carbon-copy recipients.
func (p *Pending) Cc(addresses ...string) *Pending {
	p.cc = append(p.cc, addresses...)
	return p
}

// Bcc adds blind carbon-copy recipients.
func (p *Pending) Bcc(addresses ...string) *Pending {
	p.bcc = append(p.bcc, addresses...)
	return p
}

// Using renders the mailable's views through this renderer.
func (p *Pending) Using(renderer Renderer) *Pending {
	p.renderer = renderer
	return p
}

// Send builds the mailable and delivers it to the pending recipients,
// which are added to whatever the envelope already names.
func (p *Pending) Send(mailable Mailable) error {
	message, err := Build(mailable, p.renderer)
	if err != nil {
		return err
	}

	message.To(p.to...)
	message.Cc(p.cc...)
	message.Bcc(p.bcc...)

	return p.mailer.Send(message)
}
