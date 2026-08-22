package notifications

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Broadcaster delivers a payload to a channel. broadcast.Hub satisfies
// it, so a notification can reach a connected browser.
type Broadcaster interface {
	Broadcast(channel, event string, payload any) error
}

// BroadcastMessage is what a notification pushes over websockets. An
// empty Channel uses the notifiable's route for the "broadcast" channel.
type BroadcastMessage struct {
	Channel string
	Event   string
	Payload map[string]any
}

// BroadcastNotification provides the broadcast channel's message.
type BroadcastNotification interface {
	ToBroadcast(notifiable Notifiable) BroadcastMessage
}

// WebhookMessage is an HTTP POST to a webhook endpoint - how a Slack or
// Teams incoming webhook is addressed. An empty URL uses the notifiable's
// route for the "webhook" channel.
type WebhookMessage struct {
	URL     string
	Payload any
	Headers map[string]string
}

// WebhookNotification provides the webhook channel's payload.
type WebhookNotification interface {
	ToWebhook(notifiable Notifiable) WebhookMessage
}

// ChannelFunc delivers a notification on a channel the framework does
// not know about.
type ChannelFunc func(notifiable Notifiable, notification Notification) error

// WithBroadcaster enables the broadcast channel.
func WithBroadcaster(broadcaster Broadcaster) Option {
	return func(m *Manager) { m.broadcaster = broadcaster }
}

// WithHTTPClient overrides the client used by the webhook channel, for
// tests or for a client with different timeouts.
func WithHTTPClient(client *http.Client) Option {
	return func(m *Manager) { m.httpClient = client }
}

// Extend registers a channel of your own, Laravel's custom notification
// channels:
//
//	manager.Extend("sms", func(n Notifiable, notification Notification) error {
//	    return twilio.Send(n.RouteNotificationFor("sms").(string), ...)
//	})
func (m *Manager) Extend(channel string, fn ChannelFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.custom == nil {
		m.custom = make(map[string]ChannelFunc)
	}
	m.custom[channel] = fn
}

// customChannel returns a registered custom channel.
func (m *Manager) customChannel(channel string) (ChannelFunc, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	fn, ok := m.custom[channel]
	return fn, ok
}

// sendBroadcast delivers over websockets.
func (m *Manager) sendBroadcast(notifiable Notifiable, notification Notification) error {
	if m.broadcaster == nil {
		return fmt.Errorf("no broadcaster configured (use notifications.WithBroadcaster)")
	}

	broadcastable, ok := notification.(BroadcastNotification)
	if !ok {
		return fmt.Errorf("%T does not implement ToBroadcast", notification)
	}

	message := broadcastable.ToBroadcast(notifiable)

	channel := message.Channel
	if channel == "" {
		routed, _ := notifiable.RouteNotificationFor("broadcast").(string)
		channel = routed
	}
	if channel == "" {
		return fmt.Errorf("no broadcast channel for %T", notifiable)
	}

	event := message.Event
	if event == "" {
		event = nameFor(notification)
	}

	return m.broadcaster.Broadcast(channel, event, message.Payload)
}

// sendWebhook posts the notification to an HTTP endpoint.
func (m *Manager) sendWebhook(notifiable Notifiable, notification Notification) error {
	hookable, ok := notification.(WebhookNotification)
	if !ok {
		return fmt.Errorf("%T does not implement ToWebhook", notification)
	}

	message := hookable.ToWebhook(notifiable)

	url := message.URL
	if url == "" {
		routed, _ := notifiable.RouteNotificationFor("webhook").(string)
		url = routed
	}
	if url == "" {
		return fmt.Errorf("no webhook URL for %T", notifiable)
	}

	body, err := json.Marshal(message.Payload)
	if err != nil {
		return fmt.Errorf("encoding the webhook payload: %w", err)
	}

	request, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	for key, value := range message.Headers {
		request.Header.Set(key, value)
	}

	response, err := m.client().Do(request)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()

	// A 5xx from the endpoint means the notification was not delivered;
	// reporting success here would hide a dropped alert.
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("webhook returned %s", response.Status)
	}
	return nil
}

// client returns the HTTP client the webhook channel posts with.
func (m *Manager) client() *http.Client {
	if m.httpClient != nil {
		return m.httpClient
	}
	return defaultWebhookClient
}

// defaultWebhookClient bounds a webhook post: a hung endpoint must not
// wedge the request or worker sending the notification.
var defaultWebhookClient = &http.Client{Timeout: 10 * time.Second}
