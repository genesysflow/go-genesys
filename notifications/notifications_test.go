package notifications_test

import (
	"database/sql"
	"testing"

	"github.com/genesysflow/go-genesys/mail"
	"github.com/genesysflow/go-genesys/notifications"
	"github.com/genesysflow/go-genesys/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

type customer struct {
	ID    int64
	Email string
}

func (c *customer) RouteNotificationFor(channel string) any {
	switch channel {
	case "mail":
		return c.Email
	case "database":
		return c.ID
	}
	return nil
}

type invoicePaid struct {
	Amount   int
	channels []string
}

func (n *invoicePaid) Via(_ notifications.Notifiable) []string { return n.channels }
func (n *invoicePaid) ToMail(_ notifications.Notifiable) *mail.Message {
	return mail.NewMessage().Subject("Invoice paid").Text("Thanks!")
}
func (n *invoicePaid) ToDatabase(_ notifications.Notifiable) map[string]any {
	return map[string]any{"amount": n.Amount}
}

type sqliteExecutor struct{ db *sql.DB }

func (e *sqliteExecutor) Query(q string, b ...any) (*sql.Rows, error) { return e.db.Query(q, b...) }
func (e *sqliteExecutor) QueryRow(q string, b ...any) *sql.Row        { return e.db.QueryRow(q, b...) }
func (e *sqliteExecutor) Exec(q string, b ...any) (sql.Result, error) { return e.db.Exec(q, b...) }

func newNotificationDB(t *testing.T) query.Executor {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	_, err = db.Exec(`CREATE TABLE notifications (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		notifiable_id TEXT NOT NULL,
		name TEXT NOT NULL,
		data TEXT NOT NULL,
		read_at TIMESTAMP,
		created_at TIMESTAMP NOT NULL)`)
	require.NoError(t, err)
	return &sqliteExecutor{db: db}
}

func TestMailChannel(t *testing.T) {
	mailer := mail.NewArrayMailer(mail.Config{FromAddress: "app@example.com"})
	manager := notifications.New(notifications.WithMailer(mailer))

	user := &customer{ID: 1, Email: "user@example.com"}
	require.NoError(t, manager.Send(user, &invoicePaid{Amount: 100, channels: []string{"mail"}}))

	mailer.AssertSentCount(t, 1)
	mailer.AssertSentTo(t, "user@example.com")
	sent := mailer.Sent()
	assert.Equal(t, "Invoice paid", sent[0].GetSubject())
}

func TestDatabaseChannelLifecycle(t *testing.T) {
	executor := newNotificationDB(t)
	manager := notifications.New(notifications.WithDatabase("sqlite", executor, ""))

	user := &customer{ID: 7, Email: "user@example.com"}
	require.NoError(t, manager.Send(user, &invoicePaid{Amount: 100, channels: []string{"database"}}))
	require.NoError(t, manager.Send(user, &invoicePaid{Amount: 250, channels: []string{"database"}}))

	unread, err := manager.UnreadFor(user.ID)
	require.NoError(t, err)
	require.Len(t, unread, 2)
	assert.Equal(t, "invoice_paid", unread[0].Name)
	assert.EqualValues(t, 250, unread[0].Data["amount"], "newest first")
	assert.EqualValues(t, 100, unread[1].Data["amount"])

	require.NoError(t, manager.MarkAsRead(unread[1].ID))
	unread, err = manager.UnreadFor(user.ID)
	require.NoError(t, err)
	assert.Len(t, unread, 1)

	all, err := manager.For(user.ID)
	require.NoError(t, err)
	assert.Len(t, all, 2, "read notifications still listed by For")
	for _, item := range all {
		assert.False(t, item.CreatedAt.IsZero(), "CreatedAt populated from the row")
	}
	readCount := 0
	for _, item := range all {
		if item.ReadAt != nil {
			readCount++
		}
	}
	assert.Equal(t, 1, readCount, "the marked notification carries its ReadAt")

	require.NoError(t, manager.MarkAllAsRead(user.ID))
	unread, err = manager.UnreadFor(user.ID)
	require.NoError(t, err)
	assert.Empty(t, unread)

	// Other notifiables see nothing.
	other, err := manager.For(99)
	require.NoError(t, err)
	assert.Empty(t, other)
}

func TestMultiChannelAndOnDemand(t *testing.T) {
	mailer := mail.NewArrayMailer(mail.Config{FromAddress: "app@example.com"})
	executor := newNotificationDB(t)
	manager := notifications.New(
		notifications.WithMailer(mailer),
		notifications.WithDatabase("sqlite", executor, ""),
	)

	user := &customer{ID: 3, Email: "multi@example.com"}
	require.NoError(t, manager.Send(user, &invoicePaid{Amount: 9, channels: []string{"mail", "database"}}))
	mailer.AssertSentCount(t, 1)
	stored, err := manager.For(user.ID)
	require.NoError(t, err)
	assert.Len(t, stored, 1)

	// On-demand route without a model.
	require.NoError(t, manager.Send(
		notifications.Route("mail", "ops@example.com"),
		&invoicePaid{Amount: 1, channels: []string{"mail"}},
	))
	mailer.AssertSentTo(t, "ops@example.com")

	// A notifiable that does not route a channel is skipped silently.
	require.NoError(t, manager.Send(
		notifications.Route("database", 42),
		&invoicePaid{Amount: 1, channels: []string{"mail"}},
	))
	mailer.AssertSentCount(t, 2)
}

func TestChannelErrors(t *testing.T) {
	manager := notifications.New() // nothing configured

	user := &customer{ID: 1, Email: "x@y.z"}
	assert.ErrorContains(t, manager.Send(user, &invoicePaid{channels: []string{"mail"}}), "no mailer")
	assert.ErrorContains(t, manager.Send(user, &invoicePaid{channels: []string{"database"}}), "no database")
	assert.ErrorContains(t, manager.Send(user, &invoicePaid{channels: []string{"sms"}}), "unknown channel")
}
