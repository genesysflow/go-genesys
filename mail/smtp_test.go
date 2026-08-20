package mail_test

import (
	"bufio"
	"net"
	"strings"
	"sync"
	"testing"

	"github.com/genesysflow/go-genesys/mail"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeSMTPServer speaks just enough SMTP to accept one message.
type fakeSMTPServer struct {
	listener net.Listener
	mu       sync.Mutex
	from     string
	rcpts    []string
	data     string
	authed   bool
}

func newFakeSMTPServer(t *testing.T) *fakeSMTPServer {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	server := &fakeSMTPServer{listener: listener}
	t.Cleanup(func() { listener.Close() })

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		server.serve(conn)
	}()
	return server
}

func (s *fakeSMTPServer) port() int {
	return s.listener.Addr().(*net.TCPAddr).Port
}

func (s *fakeSMTPServer) serve(conn net.Conn) {
	reader := bufio.NewReader(conn)
	write := func(line string) { conn.Write([]byte(line + "\r\n")) }

	write("220 fake.test ESMTP")
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		upper := strings.ToUpper(line)

		switch {
		case strings.HasPrefix(upper, "EHLO"), strings.HasPrefix(upper, "HELO"):
			write("250-fake.test")
			write("250 AUTH PLAIN")
		case strings.HasPrefix(upper, "AUTH PLAIN"):
			s.mu.Lock()
			s.authed = true
			s.mu.Unlock()
			write("235 ok")
		case strings.HasPrefix(upper, "MAIL FROM:"):
			s.mu.Lock()
			s.from = line[len("MAIL FROM:"):]
			s.mu.Unlock()
			write("250 ok")
		case strings.HasPrefix(upper, "RCPT TO:"):
			s.mu.Lock()
			s.rcpts = append(s.rcpts, line[len("RCPT TO:"):])
			s.mu.Unlock()
			write("250 ok")
		case upper == "DATA":
			write("354 go ahead")
			var body strings.Builder
			for {
				dataLine, err := reader.ReadString('\n')
				if err != nil {
					return
				}
				if strings.TrimRight(dataLine, "\r\n") == "." {
					break
				}
				body.WriteString(dataLine)
			}
			s.mu.Lock()
			s.data = body.String()
			s.mu.Unlock()
			write("250 queued")
		case upper == "QUIT":
			write("221 bye")
			return
		default:
			write("250 ok")
		}
	}
}

func TestSMTPMailerDeliversMessage(t *testing.T) {
	server := newFakeSMTPServer(t)

	mailer := mail.NewSMTPMailer(mail.Config{
		Host:        "127.0.0.1",
		Port:        server.port(),
		Encryption:  "none",
		Username:    "user",
		Password:    "pass",
		FromAddress: "app@example.com",
		FromName:    "App",
	})

	message := mail.NewMessage().
		To("to@example.com").
		Bcc("hidden@example.com").
		Subject("Hello SMTP").
		Text("body text")

	require.NoError(t, mailer.Send(message))

	server.mu.Lock()
	defer server.mu.Unlock()
	assert.True(t, server.authed, "credentials should be sent when configured")
	assert.Contains(t, server.from, "app@example.com")
	// Both the To and the Bcc recipient appear in the envelope...
	assert.Len(t, server.rcpts, 2)
	assert.Contains(t, strings.Join(server.rcpts, ","), "hidden@example.com")
	// ...but the Bcc never appears in the message itself.
	assert.Contains(t, server.data, "Subject: Hello SMTP")
	assert.Contains(t, server.data, "To: to@example.com")
	assert.NotContains(t, server.data, "hidden@example.com")
}

func TestSMTPMailerConnectionRefused(t *testing.T) {
	// Grab a free port and close it so nothing is listening.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	mailer := mail.NewSMTPMailer(mail.Config{
		Host:        "127.0.0.1",
		Port:        port,
		Encryption:  "none",
		FromAddress: "app@example.com",
	})
	err = mailer.Send(mail.NewMessage().To("x@y.z").Subject("s").Text("b"))
	assert.ErrorContains(t, err, "SMTP connection failed")
}

func TestSMTPMailerRequiresStartTLSWhenConfigured(t *testing.T) {
	server := newFakeSMTPServer(t) // does not advertise STARTTLS

	mailer := mail.NewSMTPMailer(mail.Config{
		Host:        "127.0.0.1",
		Port:        server.port(),
		Encryption:  "starttls",
		FromAddress: "app@example.com",
	})
	err := mailer.Send(mail.NewMessage().To("x@y.z").Subject("s").Text("b"))
	assert.ErrorContains(t, err, "STARTTLS")
}
