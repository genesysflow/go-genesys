package providers

import (
	"testing"

	facadenotify "github.com/genesysflow/go-genesys/facades/notify"
	"github.com/genesysflow/go-genesys/mail"
	"github.com/genesysflow/go-genesys/notifications"
	"github.com/genesysflow/go-genesys/testutil"
	"github.com/stretchr/testify/require"
)

type notifTestUser struct{ Email string }

func (u *notifTestUser) RouteNotificationFor(channel string) any {
	if channel == "mail" {
		return u.Email
	}
	return nil
}

type notifTestPing struct{}

func (n *notifTestPing) Via(_ notifications.Notifiable) []string { return []string{"mail"} }
func (n *notifTestPing) ToMail(_ notifications.Notifiable) *mail.Message {
	return mail.NewMessage().Subject("ping").Text("pong")
}

func TestNotificationServiceProvider(t *testing.T) {
	app := testutil.NewMockApplication()
	mailer := mail.NewArrayMailer(mail.Config{FromAddress: "app@example.com"})
	require.NoError(t, app.Instance("mailer", mailer))

	provider := &NotificationServiceProvider{}
	require.NoError(t, provider.Register(app))
	require.NoError(t, provider.Boot(app))
	t.Cleanup(func() { facadenotify.SetInstance(nil) })

	manager, ok := app.GetInstance("notifications").(*notifications.Manager)
	require.True(t, ok)

	require.NoError(t, manager.Send(&notifTestUser{Email: "u@x.io"}, &notifTestPing{}))
	mailer.AssertSentTo(t, "u@x.io")

	// The facade was wired too.
	require.NoError(t, facadenotify.Send(&notifTestUser{Email: "v@x.io"}, &notifTestPing{}))
	mailer.AssertSentCount(t, 2)
}
