package mail

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strconv"
	"sync"

	"github.com/genesysflow/go-genesys/contracts"
)

// Mailer sends messages.
type Mailer interface {
	// Send delivers a message.
	Send(message *Message) error
}

// Config configures the mail system (config/mail.yaml).
type Config struct {
	// Driver is the mail driver: smtp, log, or array.
	Driver string `yaml:"driver" json:"driver"`

	// Host is the SMTP server host.
	Host string `yaml:"host" json:"host"`

	// Port is the SMTP server port (default 587).
	Port int `yaml:"port" json:"port"`

	// Username for SMTP authentication (empty = no auth).
	Username string `yaml:"username" json:"username"`

	// Password for SMTP authentication.
	Password string `yaml:"password" json:"password"`

	// Encryption: "tls" (implicit TLS, port 465), "starttls" (default),
	// or "none".
	Encryption string `yaml:"encryption" json:"encryption"`

	// FromAddress is the default sender address.
	FromAddress string `yaml:"from_address" json:"from_address"`

	// FromName is the default sender display name.
	FromName string `yaml:"from_name" json:"from_name"`
}

// SMTPMailer delivers messages over SMTP.
type SMTPMailer struct {
	config Config
}

// NewSMTPMailer creates an SMTP mailer.
func NewSMTPMailer(config Config) *SMTPMailer {
	if config.Port == 0 {
		config.Port = 587
	}
	if config.Encryption == "" {
		config.Encryption = "starttls"
	}
	return &SMTPMailer{config: config}
}

// Send delivers the message via SMTP.
func (m *SMTPMailer) Send(message *Message) error {
	applyDefaults(message, m.config)

	raw, err := message.Bytes()
	if err != nil {
		return err
	}

	addr := net.JoinHostPort(m.config.Host, strconv.Itoa(m.config.Port))

	var client *smtp.Client
	switch m.config.Encryption {
	case "tls":
		conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: m.config.Host})
		if err != nil {
			return fmt.Errorf("mail: TLS connection failed: %w", err)
		}
		client, err = smtp.NewClient(conn, m.config.Host)
		if err != nil {
			return err
		}
	default:
		client, err = smtp.Dial(addr)
		if err != nil {
			return fmt.Errorf("mail: SMTP connection failed: %w", err)
		}
		if m.config.Encryption == "starttls" {
			if ok, _ := client.Extension("STARTTLS"); !ok {
				client.Close()
				return fmt.Errorf("mail: server does not support STARTTLS (set encryption: none to allow plaintext)")
			}
			if err := client.StartTLS(&tls.Config{ServerName: m.config.Host}); err != nil {
				client.Close()
				return err
			}
		}
	}
	defer client.Close()

	if m.config.Username != "" {
		auth := smtp.PlainAuth("", m.config.Username, m.config.Password, m.config.Host)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("mail: SMTP auth failed: %w", err)
		}
	}

	if err := client.Mail(message.FromAddress()); err != nil {
		return err
	}
	for _, rcpt := range message.Recipients() {
		if err := client.Rcpt(rcpt); err != nil {
			return fmt.Errorf("mail: recipient %s rejected: %w", rcpt, err)
		}
	}

	writer, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := writer.Write(raw); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	return client.Quit()
}

// LogMailer writes messages to the application log instead of sending
// them; the development default.
type LogMailer struct {
	config Config
	logger contracts.Logger
}

// NewLogMailer creates a log mailer.
func NewLogMailer(config Config, logger contracts.Logger) *LogMailer {
	return &LogMailer{config: config, logger: logger}
}

// Send logs the rendered message.
func (m *LogMailer) Send(message *Message) error {
	applyDefaults(message, m.config)
	raw, err := message.Bytes()
	if err != nil {
		return err
	}
	if m.logger != nil {
		m.logger.WithFields(map[string]any{
			"to":      message.GetTo(),
			"subject": message.GetSubject(),
		}).Info("Mail message logged (log driver)")
		m.logger.Debug(string(raw))
	}
	return nil
}

// ArrayMailer captures sent messages in memory for tests.
type ArrayMailer struct {
	config   Config
	mu       sync.Mutex
	messages []*Message
}

// NewArrayMailer creates an array mailer.
func NewArrayMailer(config ...Config) *ArrayMailer {
	cfg := Config{}
	if len(config) > 0 {
		cfg = config[0]
	}
	return &ArrayMailer{config: cfg}
}

// Send validates and captures the message.
func (m *ArrayMailer) Send(message *Message) error {
	applyDefaults(message, m.config)
	if _, err := message.Bytes(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, message)
	return nil
}

// Sent returns the captured messages.
func (m *ArrayMailer) Sent() []*Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]*Message(nil), m.messages...)
}

// applyDefaults fills the sender from config when the message has none.
func applyDefaults(message *Message, config Config) {
	if message.from == "" && config.FromAddress != "" {
		message.From(config.FromAddress, config.FromName)
	}
}
