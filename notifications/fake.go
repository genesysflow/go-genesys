package notifications

import (
	"sync"
)

// TestingT is the subset of *testing.T the assertions use.
type TestingT interface {
	Helper()
	Errorf(format string, args ...any)
}

// SentNotification records one delivery made through a Fake.
type SentNotification struct {
	Notifiable   Notifiable
	Notification Notification
}

// Fake records notifications instead of delivering them, so a test can
// assert what an action would have sent without a mail server or an
// HTTP endpoint - Laravel's Notification::fake().
type Fake struct {
	mu   sync.Mutex
	sent []SentNotification
}

// NewFake creates a recording notification sender.
func NewFake() *Fake {
	return &Fake{}
}

// Send records the notification.
func (f *Fake) Send(notifiable Notifiable, notification Notification) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, SentNotification{Notifiable: notifiable, Notification: notification})
	return nil
}

// Sent returns everything recorded.
func (f *Fake) Sent() []SentNotification {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]SentNotification(nil), f.sent...)
}

// Clear forgets what was recorded.
func (f *Fake) Clear() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = nil
}

// AssertSentCount fails unless exactly expected notifications were sent.
func (f *Fake) AssertSentCount(t TestingT, expected int) {
	t.Helper()
	if actual := len(f.Sent()); actual != expected {
		t.Errorf("expected %d notifications to be sent, got %d", expected, actual)
	}
}

// AssertNothingSent fails when any notification was sent.
func (f *Fake) AssertNothingSent(t TestingT) {
	t.Helper()
	if sent := f.Sent(); len(sent) > 0 {
		t.Errorf("expected no notifications to be sent, got %d", len(sent))
	}
}

// AssertSentTo fails unless a notification went to an address the
// notifiable routes to on any channel.
func (f *Fake) AssertSentTo(t TestingT, address any) {
	t.Helper()

	for _, record := range f.Sent() {
		for _, channel := range record.Notification.Via(record.Notifiable) {
			if record.Notifiable.RouteNotificationFor(channel) == address {
				return
			}
		}
	}
	t.Errorf("expected a notification addressed to %v", address)
}

// AssertSent fails unless a notification of type N was sent, optionally
// matching a predicate:
//
//	notifications.AssertSent[InvoicePaid](t, fake, func(n *InvoicePaid) bool {
//	    return n.Amount == 100
//	})
func AssertSent[N any](t TestingT, fake *Fake, match ...func(*N) bool) []*N {
	t.Helper()

	var found []*N
	for _, record := range fake.Sent() {
		typed, ok := any(record.Notification).(*N)
		if !ok {
			continue
		}
		if len(match) > 0 && !match[0](typed) {
			continue
		}
		found = append(found, typed)
	}

	if len(found) == 0 {
		t.Errorf("expected a matching notification to be sent, none was")
	}
	return found
}

// AssertNotSent fails when a notification of type N was sent.
func AssertNotSent[N any](t TestingT, fake *Fake) {
	t.Helper()

	for _, record := range fake.Sent() {
		if _, ok := any(record.Notification).(*N); ok {
			t.Errorf("expected no %T notification to be sent, one was", record.Notification)
			return
		}
	}
}
