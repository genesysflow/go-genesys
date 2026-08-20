package notifications_test

import (
	"testing"

	"github.com/genesysflow/go-genesys/mail"
	"github.com/genesysflow/go-genesys/notifications"
	"github.com/genesysflow/go-genesys/queue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type paymentReceived struct {
	Amount int `json:"amount"`
}

func (n *paymentReceived) Via(_ notifications.Notifiable) []string {
	return []string{"mail", "database"}
}
func (n *paymentReceived) ToMail(_ notifications.Notifiable) *mail.Message {
	return mail.NewMessage().Subject("Payment received").Text("thanks")
}
func (n *paymentReceived) ToDatabase(_ notifications.Notifiable) map[string]any {
	return map[string]any{"amount": n.Amount}
}

type unregisteredNote struct{}

func (n *unregisteredNote) Via(_ notifications.Notifiable) []string { return []string{"mail"} }
func (n *unregisteredNote) ToMail(_ notifications.Notifiable) *mail.Message {
	return mail.NewMessage().Subject("x").Text("y")
}

func TestQueuedNotificationRoundTrip(t *testing.T) {
	notifications.RegisterQueued[paymentReceived]()

	mailer := mail.NewArrayMailer(mail.Config{FromAddress: "app@example.com"})
	executor := newNotificationDB(t)
	manager := notifications.New(
		notifications.WithMailer(mailer),
		notifications.WithDatabase("sqlite", executor, ""),
	)
	notifications.SetDefault(manager)
	t.Cleanup(func() { notifications.SetDefault(nil) })

	q := queue.NewMemoryQueue()
	user := &customer{ID: 9, Email: "queued@example.com"}

	require.NoError(t, manager.SendQueued(q, user, &paymentReceived{Amount: 250}))

	// Nothing delivered until a worker runs.
	mailer.AssertNothingSent(t)
	assert.Equal(t, 1, q.Size(""))

	worker := queue.NewWorker(q)
	require.NoError(t, worker.Drain())

	// The worker delivered on both channels with dispatch-time routes.
	mailer.AssertSentTo(t, "queued@example.com")
	stored, err := manager.For(user.ID)
	require.NoError(t, err)
	require.Len(t, stored, 1)
	assert.EqualValues(t, 250, stored[0].Data["amount"], "payload survived serialization")
}

func TestSendQueuedRequiresRegistration(t *testing.T) {
	q := queue.NewMemoryQueue()
	manager := notifications.New()
	err := manager.SendQueued(q, &customer{ID: 1, Email: "a@x.io"}, &unregisteredNote{})
	assert.ErrorContains(t, err, "not registered for queueing")
	assert.Equal(t, 0, q.Size(""))
}

func TestSendQueuedSkipsUnroutedNotifiables(t *testing.T) {
	notifications.RegisterQueued[paymentReceived]()
	q := queue.NewMemoryQueue()
	manager := notifications.New()

	// A notifiable that routes neither channel dispatches nothing.
	require.NoError(t, manager.SendQueued(q, notifications.Route("sms", "+1555"), &paymentReceived{}))
	assert.Equal(t, 0, q.Size(""))
}

func TestQueuedJobFailsWithoutDefaultManager(t *testing.T) {
	notifications.RegisterQueued[paymentReceived]()
	notifications.SetDefault(nil)

	q := queue.NewMemoryQueue()
	manager := notifications.New(notifications.WithMailer(mail.NewArrayMailer()))
	user := &customer{ID: 2, Email: "b@x.io"}
	require.NoError(t, manager.SendQueued(q, user, &paymentReceived{Amount: 1}))

	worker := queue.NewWorker(q)
	worker.Tries = 1
	require.NoError(t, worker.Drain())

	failed, err := q.ListFailed()
	require.NoError(t, err)
	require.Len(t, failed, 1)
	assert.Contains(t, failed[0].Exception, "no default manager")
}
