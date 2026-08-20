// Package mail provides Laravel-style mail sending with SMTP and log
// drivers.
package mail

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"mime"
	"mime/multipart"
	"net/textproto"
	"strings"
	"time"
)

// Message is an email under construction:
//
//	message := mail.NewMessage().
//	    To("user@example.com").
//	    Subject("Welcome!").
//	    HTML("<h1>Hello</h1>").
//	    Text("Hello")
type Message struct {
	from        string
	fromName    string
	to          []string
	cc          []string
	bcc         []string
	replyTo     string
	subject     string
	textBody    string
	htmlBody    string
	attachments []attachment
	headers     map[string]string
}

type attachment struct {
	filename    string
	contentType string
	content     []byte
}

// NewMessage creates an empty message.
func NewMessage() *Message {
	return &Message{headers: make(map[string]string)}
}

// From sets the sender address (and optional display name).
func (m *Message) From(address string, name ...string) *Message {
	m.from = address
	if len(name) > 0 {
		m.fromName = name[0]
	}
	return m
}

// To adds recipient addresses.
func (m *Message) To(addresses ...string) *Message {
	m.to = append(m.to, addresses...)
	return m
}

// Cc adds carbon-copy addresses.
func (m *Message) Cc(addresses ...string) *Message {
	m.cc = append(m.cc, addresses...)
	return m
}

// Bcc adds blind carbon-copy addresses.
func (m *Message) Bcc(addresses ...string) *Message {
	m.bcc = append(m.bcc, addresses...)
	return m
}

// ReplyTo sets the reply-to address.
func (m *Message) ReplyTo(address string) *Message {
	m.replyTo = address
	return m
}

// Subject sets the subject line.
func (m *Message) Subject(subject string) *Message {
	m.subject = subject
	return m
}

// Text sets the plain-text body.
func (m *Message) Text(body string) *Message {
	m.textBody = body
	return m
}

// HTML sets the HTML body.
func (m *Message) HTML(body string) *Message {
	m.htmlBody = body
	return m
}

// Attach adds a file attachment.
func (m *Message) Attach(filename string, content []byte, contentType ...string) *Message {
	ct := "application/octet-stream"
	if len(contentType) > 0 {
		ct = contentType[0]
	}
	m.attachments = append(m.attachments, attachment{filename: filename, contentType: ct, content: content})
	return m
}

// Header sets a custom header.
func (m *Message) Header(key, value string) *Message {
	m.headers[key] = value
	return m
}

// Recipients returns all envelope recipients (to + cc + bcc).
func (m *Message) Recipients() []string {
	out := make([]string, 0, len(m.to)+len(m.cc)+len(m.bcc))
	out = append(out, m.to...)
	out = append(out, m.cc...)
	out = append(out, m.bcc...)
	return out
}

// FromAddress returns the sender address.
func (m *Message) FromAddress() string { return m.from }

// GetSubject returns the subject line.
func (m *Message) GetSubject() string { return m.subject }

// GetTo returns the To recipients.
func (m *Message) GetTo() []string { return append([]string(nil), m.to...) }

// GetHTML returns the HTML body.
func (m *Message) GetHTML() string { return m.htmlBody }

// GetText returns the plain-text body.
func (m *Message) GetText() string { return m.textBody }

// validate checks the message can be sent.
func (m *Message) validate() error {
	if m.from == "" {
		return fmt.Errorf("mail: message has no from address")
	}
	if len(m.Recipients()) == 0 {
		return fmt.Errorf("mail: message has no recipients")
	}
	if m.textBody == "" && m.htmlBody == "" {
		return fmt.Errorf("mail: message has no body")
	}
	return nil
}

// Bytes renders the full RFC 5322 message (headers + MIME body). Bcc
// addresses are intentionally not rendered into headers.
func (m *Message) Bytes() ([]byte, error) {
	if err := m.validate(); err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	writeHeader := func(key, value string) {
		fmt.Fprintf(&buf, "%s: %s\r\n", key, value)
	}

	fromHeader := m.from
	if m.fromName != "" {
		fromHeader = fmt.Sprintf("%s <%s>", mime.QEncoding.Encode("utf-8", m.fromName), m.from)
	}
	writeHeader("From", fromHeader)
	writeHeader("To", strings.Join(m.to, ", "))
	if len(m.cc) > 0 {
		writeHeader("Cc", strings.Join(m.cc, ", "))
	}
	if m.replyTo != "" {
		writeHeader("Reply-To", m.replyTo)
	}
	writeHeader("Subject", mime.QEncoding.Encode("utf-8", m.subject))
	writeHeader("Date", time.Now().Format(time.RFC1123Z))
	writeHeader("MIME-Version", "1.0")
	for key, value := range m.headers {
		writeHeader(key, value)
	}

	body := multipart.NewWriter(&buf)

	if len(m.attachments) > 0 {
		writeHeader("Content-Type", `multipart/mixed; boundary="`+body.Boundary()+`"`)
		buf.WriteString("\r\n")
		if err := m.writeBodyPart(body); err != nil {
			return nil, err
		}
		for _, att := range m.attachments {
			header := textproto.MIMEHeader{}
			header.Set("Content-Type", att.contentType)
			header.Set("Content-Transfer-Encoding", "base64")
			header.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, att.filename))
			part, err := body.CreatePart(header)
			if err != nil {
				return nil, err
			}
			part.Write(wrapBase64(att.content))
		}
		body.Close()
		return buf.Bytes(), nil
	}

	if m.textBody != "" && m.htmlBody != "" {
		writeHeader("Content-Type", `multipart/alternative; boundary="`+body.Boundary()+`"`)
		buf.WriteString("\r\n")
		if err := writeTextPart(body, "text/plain", m.textBody); err != nil {
			return nil, err
		}
		if err := writeTextPart(body, "text/html", m.htmlBody); err != nil {
			return nil, err
		}
		body.Close()
		return buf.Bytes(), nil
	}

	// Single-part message.
	contentType := "text/plain"
	content := m.textBody
	if m.htmlBody != "" {
		contentType = "text/html"
		content = m.htmlBody
	}
	writeHeader("Content-Type", contentType+`; charset="utf-8"`)
	writeHeader("Content-Transfer-Encoding", "base64")
	buf.WriteString("\r\n")
	buf.Write(wrapBase64([]byte(content)))
	return buf.Bytes(), nil
}

// writeBodyPart writes the text/html bodies inside a multipart/mixed message.
func (m *Message) writeBodyPart(mixed *multipart.Writer) error {
	if m.textBody != "" && m.htmlBody != "" {
		header := textproto.MIMEHeader{}
		alt := multipart.NewWriter(nil)
		header.Set("Content-Type", `multipart/alternative; boundary="`+alt.Boundary()+`"`)
		part, err := mixed.CreatePart(header)
		if err != nil {
			return err
		}
		nested := multipart.NewWriter(part)
		nested.SetBoundary(alt.Boundary())
		if err := writeTextPart(nested, "text/plain", m.textBody); err != nil {
			return err
		}
		if err := writeTextPart(nested, "text/html", m.htmlBody); err != nil {
			return err
		}
		return nested.Close()
	}

	contentType := "text/plain"
	content := m.textBody
	if m.htmlBody != "" {
		contentType = "text/html"
		content = m.htmlBody
	}
	return writeTextPart(mixed, contentType, content)
}

func writeTextPart(w *multipart.Writer, contentType, content string) error {
	header := textproto.MIMEHeader{}
	header.Set("Content-Type", contentType+`; charset="utf-8"`)
	header.Set("Content-Transfer-Encoding", "base64")
	part, err := w.CreatePart(header)
	if err != nil {
		return err
	}
	_, err = part.Write(wrapBase64([]byte(content)))
	return err
}

// wrapBase64 encodes content in base64 with RFC-compliant 76-column lines.
func wrapBase64(content []byte) []byte {
	encoded := base64.StdEncoding.EncodeToString(content)
	var buf bytes.Buffer
	for len(encoded) > 76 {
		buf.WriteString(encoded[:76])
		buf.WriteString("\r\n")
		encoded = encoded[76:]
	}
	buf.WriteString(encoded)
	buf.WriteString("\r\n")
	return buf.Bytes()
}
