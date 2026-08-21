package mail_test

import (
	"errors"
	"testing"

	"github.com/genesysflow/go-genesys/mail"
	"github.com/genesysflow/go-genesys/queue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type failingMailer struct{ calls int }

func (m *failingMailer) Send(*mail.Message) error {
	m.calls++
	return errors.New("smtp down")
}

func TestManagerNamedMailers(t *testing.T) {
	primary := mail.NewArrayMailer(mail.Config{FromAddress: "a@x.io"})
	audit := mail.NewArrayMailer(mail.Config{FromAddress: "a@x.io"})

	manager := mail.NewManager()
	manager.Register("smtp", primary)
	manager.Register("audit", audit)

	msg := mail.NewMessage().From("a@x.io").To("u@x.io").Subject("hi").Text("b")
	require.NoError(t, manager.Send(msg)) // default = first registered

	named, err := manager.Mailer("audit")
	require.NoError(t, err)
	require.NoError(t, named.Send(msg))

	primary.AssertSentCount(t, 1)
	audit.AssertSentCount(t, 1)

	_, err = manager.Mailer("missing")
	assert.ErrorContains(t, err, "not registered")
}

func TestFailoverMailer(t *testing.T) {
	broken := &failingMailer{}
	backup := mail.NewArrayMailer(mail.Config{FromAddress: "a@x.io"})
	failover := mail.NewFailoverMailer(broken, backup)

	msg := mail.NewMessage().From("a@x.io").To("u@x.io").Subject("hi").Text("b")
	require.NoError(t, failover.Send(msg))
	assert.Equal(t, 1, broken.calls, "the primary was tried first")
	backup.AssertSentCount(t, 1)

	// All failing surfaces the joined errors.
	allBroken := mail.NewFailoverMailer(&failingMailer{}, &failingMailer{})
	assert.ErrorContains(t, allBroken.Send(msg), "all failover mailers failed")
}

func TestSendQueuedMail(t *testing.T) {
	delivery := mail.NewArrayMailer(mail.Config{FromAddress: "a@x.io"})
	mail.SetDefaultMailer(delivery)
	t.Cleanup(func() { mail.SetDefaultMailer(nil) })

	q := queue.NewMemoryQueue()
	msg := mail.NewMessage().From("a@x.io").To("queued@x.io").Subject("Later").Text("body")
	require.NoError(t, mail.SendQueued(q, msg))

	delivery.AssertNothingSent(t)
	require.NoError(t, queue.NewWorker(q).Drain())
	delivery.AssertSentTo(t, "queued@x.io")
}
