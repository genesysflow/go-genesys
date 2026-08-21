package mail

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/genesysflow/go-genesys/queue"
)

// messageSnapshot is the serializable form of a Message; view bodies
// are rendered before queueing, so only concrete content travels.
type messageSnapshot struct {
	From        string             `json:"from"`
	FromName    string             `json:"from_name,omitempty"`
	To          []string           `json:"to"`
	Cc          []string           `json:"cc,omitempty"`
	Bcc         []string           `json:"bcc,omitempty"`
	ReplyTo     string             `json:"reply_to,omitempty"`
	Subject     string             `json:"subject"`
	TextBody    string             `json:"text_body,omitempty"`
	HTMLBody    string             `json:"html_body,omitempty"`
	Headers     map[string]string  `json:"headers,omitempty"`
	Attachments []attachmentDetail `json:"attachments,omitempty"`
}

type attachmentDetail struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Content     []byte `json:"content"`
}

// MarshalJSON serializes the message for queueing.
func (m *Message) MarshalJSON() ([]byte, error) {
	if m.viewErr != nil {
		return nil, m.viewErr
	}
	snapshot := messageSnapshot{
		From:     m.from,
		FromName: m.fromName,
		To:       m.to,
		Cc:       m.cc,
		Bcc:      m.bcc,
		ReplyTo:  m.replyTo,
		Subject:  m.subject,
		TextBody: m.textBody,
		HTMLBody: m.htmlBody,
		Headers:  m.headers,
	}
	for _, att := range m.attachments {
		snapshot.Attachments = append(snapshot.Attachments, attachmentDetail{
			Filename:    att.filename,
			ContentType: att.contentType,
			Content:     att.content,
		})
	}
	return json.Marshal(snapshot)
}

// UnmarshalJSON restores a queued message.
func (m *Message) UnmarshalJSON(data []byte) error {
	var snapshot messageSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return err
	}
	m.from = snapshot.From
	m.fromName = snapshot.FromName
	m.to = snapshot.To
	m.cc = snapshot.Cc
	m.bcc = snapshot.Bcc
	m.replyTo = snapshot.ReplyTo
	m.subject = snapshot.Subject
	m.textBody = snapshot.TextBody
	m.htmlBody = snapshot.HTMLBody
	m.headers = snapshot.Headers
	if m.headers == nil {
		m.headers = make(map[string]string)
	}
	m.attachments = nil
	for _, att := range snapshot.Attachments {
		m.attachments = append(m.attachments, attachment{
			filename:    att.Filename,
			contentType: att.ContentType,
			content:     att.Content,
		})
	}
	return nil
}

// --- the queue-side delivery path ---

var (
	defaultMailerMu sync.RWMutex
	defaultMailer   Mailer
)

// SetDefaultMailer installs the mailer queued messages deliver through;
// the MailServiceProvider wires it automatically.
func SetDefaultMailer(mailer Mailer) {
	defaultMailerMu.Lock()
	defer defaultMailerMu.Unlock()
	defaultMailer = mailer
}

// queuedMailJob delivers a serialized message on a worker.
type queuedMailJob struct {
	Message *Message `json:"message"`
}

// JobName gives the job a stable registered name.
func (j *queuedMailJob) JobName() string { return "genesys.mail" }

// Handle sends the message through the process's default mailer.
func (j *queuedMailJob) Handle() error {
	defaultMailerMu.RLock()
	mailer := defaultMailer
	defaultMailerMu.RUnlock()
	if mailer == nil {
		return fmt.Errorf("mail: no default mailer for queued delivery - register the MailServiceProvider or call mail.SetDefaultMailer in the worker")
	}
	if j.Message == nil {
		return fmt.Errorf("mail: queued job carries no message")
	}
	return mailer.Send(j.Message)
}

func init() {
	queue.Register[queuedMailJob]()
}

// SendQueued pushes the message onto the queue; a worker delivers it
// through the default mailer - Laravel's Mail::queue(). Render any view
// bodies before queueing (ViewHTML runs at build time, so the usual
// fluent chain just works):
//
//	mail.SendQueued(q, mail.NewMessage().To(user.Email).Subject("Hi").Text("..."))
func SendQueued(q queue.Queue, message *Message) error {
	if message.viewErr != nil {
		return message.viewErr
	}
	if err := message.validate(); err != nil {
		return err
	}
	return q.Push(&queuedMailJob{Message: message})
}
