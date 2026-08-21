// Package notify provides a static-style accessor for the notification
// manager, following the same pattern as the other facades.
package notify

import (
	"sync"

	"github.com/genesysflow/go-genesys/notifications"
	"github.com/genesysflow/go-genesys/queue"
)

var (
	mu       sync.RWMutex
	instance *notifications.Manager
)

// SetInstance sets the manager the facade delegates to.
func SetInstance(manager *notifications.Manager) {
	mu.Lock()
	defer mu.Unlock()
	instance = manager
}

// GetInstance returns the current manager (nil when unset).
func GetInstance() *notifications.Manager {
	mu.RLock()
	defer mu.RUnlock()
	return instance
}

func mustInstance() *notifications.Manager {
	mu.RLock()
	defer mu.RUnlock()
	if instance == nil {
		panic("notify facade: no instance set - register the NotificationServiceProvider or call notify.SetInstance")
	}
	return instance
}

// Send delivers a notification on every channel it declares.
func Send(notifiable notifications.Notifiable, notification notifications.Notification) error {
	return mustInstance().Send(notifiable, notification)
}

// Route creates an on-demand notifiable for a single channel address.
func Route(channel string, address any) notifications.Notifiable {
	return notifications.Route(channel, address)
}

// For returns a notifiable's stored notifications, newest first.
func For(notifiableID any) ([]notifications.Stored, error) {
	return mustInstance().For(notifiableID)
}

// UnreadFor returns a notifiable's unread notifications, newest first.
func UnreadFor(notifiableID any) ([]notifications.Stored, error) {
	return mustInstance().UnreadFor(notifiableID)
}

// MarkAsRead stamps a stored notification as read.
func MarkAsRead(id int64) error {
	return mustInstance().MarkAsRead(id)
}

// MarkAllAsRead stamps all of a notifiable's notifications as read.
func MarkAllAsRead(notifiableID any) error {
	return mustInstance().MarkAllAsRead(notifiableID)
}

// SendQueued dispatches a notification onto the queue; a worker
// delivers it on every routed channel. The notification type must be
// registered with notifications.RegisterQueued.
func SendQueued(q queue.Queue, notifiable notifications.Notifiable, notification notifications.Notification) error {
	return mustInstance().SendQueued(q, notifiable, notification)
}
