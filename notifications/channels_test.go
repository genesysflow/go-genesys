package notifications_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/genesysflow/go-genesys/notifications"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordingBroadcaster stands in for the websocket hub.
type recordingBroadcaster struct {
	mu       sync.Mutex
	channels []string
	events   []string
	payloads []any
}

func (b *recordingBroadcaster) Broadcast(channel, event string, payload any) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.channels = append(b.channels, channel)
	b.events = append(b.events, event)
	b.payloads = append(b.payloads, payload)
	return nil
}

// invoiceSettled is delivered over the broadcast channel.
type invoiceSettled struct{ Amount string }

func (n *invoiceSettled) Via(notifiable notifications.Notifiable) []string {
	return []string{"broadcast"}
}

func (n *invoiceSettled) ToBroadcast(notifiable notifications.Notifiable) notifications.BroadcastMessage {
	return notifications.BroadcastMessage{
		Event:   "invoice.paid",
		Payload: map[string]any{"amount": n.Amount},
	}
}

func TestBroadcastChannel(t *testing.T) {
	broadcaster := &recordingBroadcaster{}
	manager := notifications.New(notifications.WithBroadcaster(broadcaster))

	// The notifiable's route for the channel names where it goes.
	require.NoError(t, manager.Send(
		notifications.Route("broadcast", "private-users.1"),
		&invoiceSettled{Amount: "9.99"},
	))

	require.Len(t, broadcaster.channels, 1)
	assert.Equal(t, "private-users.1", broadcaster.channels[0])
	assert.Equal(t, "invoice.paid", broadcaster.events[0])
	assert.Equal(t, map[string]any{"amount": "9.99"}, broadcaster.payloads[0])
}

// A notification may name its own channel, overriding the route.
type systemAlert struct{}

func (n *systemAlert) Via(notifiable notifications.Notifiable) []string {
	return []string{"broadcast"}
}

func (n *systemAlert) ToBroadcast(notifiable notifications.Notifiable) notifications.BroadcastMessage {
	return notifications.BroadcastMessage{
		Channel: "ops",
		Event:   "system.alert",
		Payload: map[string]any{"level": "warning"},
	}
}

func TestBroadcastChannelOverride(t *testing.T) {
	broadcaster := &recordingBroadcaster{}
	manager := notifications.New(notifications.WithBroadcaster(broadcaster))

	require.NoError(t, manager.Send(notifications.Route("broadcast", "ignored"), &systemAlert{}))
	assert.Equal(t, "ops", broadcaster.channels[0])
}

func TestBroadcastChannelWithoutBroadcaster(t *testing.T) {
	manager := notifications.New()

	err := manager.Send(notifications.Route("broadcast", "users.1"), &invoiceSettled{})
	assert.Error(t, err)
}

// --- webhook ---------------------------------------------------------

// deployFinished posts to a webhook URL, which is how a Slack or Teams
// incoming webhook is addressed.
type deployFinished struct{ Version string }

func (n *deployFinished) Via(notifiable notifications.Notifiable) []string {
	return []string{"webhook"}
}

func (n *deployFinished) ToWebhook(notifiable notifications.Notifiable) notifications.WebhookMessage {
	return notifications.WebhookMessage{
		Payload: map[string]any{"text": "Deployed " + n.Version},
	}
}

func TestWebhookChannel(t *testing.T) {
	var (
		mu       sync.Mutex
		received map[string]any
		gotAuth  string
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		gotAuth = r.Header.Get("X-Token")
		_ = json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	manager := notifications.New()

	require.NoError(t, manager.Send(
		notifications.Route("webhook", server.URL),
		&deployFinished{Version: "1.4.0"},
	))

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, "Deployed 1.4.0", received["text"])
	assert.Empty(t, gotAuth)
}

// A webhook that answers with an error status must fail the send rather
// than reporting a delivery that did not happen.
func TestWebhookChannelReportsFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	manager := notifications.New()

	err := manager.Send(notifications.Route("webhook", server.URL), &deployFinished{Version: "1.4.0"})
	assert.Error(t, err)
}

// --- custom channels -------------------------------------------------

type smsNotification struct{ Body string }

func (n *smsNotification) Via(notifiable notifications.Notifiable) []string {
	return []string{"sms"}
}

func TestExtendWithACustomChannel(t *testing.T) {
	var delivered []string

	manager := notifications.New()
	manager.Extend("sms", func(notifiable notifications.Notifiable, notification notifications.Notification) error {
		number, _ := notifiable.RouteNotificationFor("sms").(string)
		delivered = append(delivered, number+":"+notification.(*smsNotification).Body)
		return nil
	})

	require.NoError(t, manager.Send(notifications.Route("sms", "+15550100"), &smsNotification{Body: "hi"}))
	assert.Equal(t, []string{"+15550100:hi"}, delivered)
}

// An unknown channel is an error, not a silent no-op: a notification
// nobody receives is worse than one that fails loudly.
func TestUnknownChannelFails(t *testing.T) {
	manager := notifications.New()

	err := manager.Send(notifications.Route("sms", "+15550100"), &smsNotification{Body: "hi"})
	assert.Error(t, err)
}

// --- fake ------------------------------------------------------------

// passwordChanged is delivered by mail, which is how the fake's
// address assertions are used in practice.
type passwordChanged struct{ At string }

func (n *passwordChanged) Via(notifiable notifications.Notifiable) []string {
	return []string{"mail"}
}

func TestNotificationFake(t *testing.T) {
	fake := notifications.NewFake()

	require.NoError(t, fake.Send(notifications.Route("mail", "ada@example.com"), &passwordChanged{At: "monday"}))
	require.NoError(t, fake.Send(notifications.Route("mail", "grace@example.com"), &passwordChanged{At: "tuesday"}))

	fake.AssertSentCount(t, 2)
	fake.AssertSentTo(t, "ada@example.com")
	fake.AssertSentTo(t, "grace@example.com")

	notifications.AssertSent[passwordChanged](t, fake, func(n *passwordChanged) bool {
		return n.At == "tuesday"
	})
	notifications.AssertNotSent[systemAlert](t, fake)
}

func TestNotificationFakeNothingSent(t *testing.T) {
	fake := notifications.NewFake()
	fake.AssertNothingSent(t)
}
