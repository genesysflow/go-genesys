package notifications

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sync"

	"github.com/genesysflow/go-genesys/queue"
)

// Queued notifications: SendQueued resolves each channel's route at
// dispatch time (an email address, a database id - plain serializable
// values), so the job carries no model and any worker process can
// deliver it. Notification types must be registered once:
//
//	notifications.RegisterQueued[InvoicePaid]()
//	...
//	manager.SendQueued(q, user, &InvoicePaid{Amount: 100})
//
// Workers deliver through the package's default manager, which the
// NotificationServiceProvider installs at boot.

var (
	defaultMu      sync.RWMutex
	defaultManager *Manager

	queuedMu      sync.RWMutex
	queuedFactory = make(map[string]func() Notification)
)

// SetDefault installs the manager workers use to deliver queued
// notifications. The NotificationServiceProvider calls this.
func SetDefault(m *Manager) {
	defaultMu.Lock()
	defer defaultMu.Unlock()
	defaultManager = m
}

// Default returns the manager queued notifications deliver through.
func Default() *Manager {
	defaultMu.RLock()
	defer defaultMu.RUnlock()
	return defaultManager
}

// RegisterQueued registers a notification type for queued delivery, so
// workers can reconstruct it from its JSON payload.
func RegisterQueued[N any]() {
	var probe N
	notification, ok := any(&probe).(Notification)
	if !ok {
		panic(fmt.Sprintf("notifications: *%T does not implement notifications.Notification", probe))
	}
	name := nameFor(notification)
	queuedMu.Lock()
	defer queuedMu.Unlock()
	queuedFactory[name] = func() Notification {
		var instance N
		return any(&instance).(Notification)
	}
}

// queuedNotificationJob is the serializable envelope pushed onto the
// queue for one notifiable.
type queuedNotificationJob struct {
	Notification string          `json:"notification"`
	Data         json.RawMessage `json:"data"`
	Routes       map[string]any  `json:"routes"` // channel -> resolved address
}

// JobName gives the wrapper a stable registered name.
func (j *queuedNotificationJob) JobName() string { return "genesys.notification" }

// Handle reconstructs the notification and delivers it on each
// recorded channel through the default manager.
func (j *queuedNotificationJob) Handle() error {
	manager := Default()
	if manager == nil {
		return fmt.Errorf("notifications: no default manager (call notifications.SetDefault)")
	}

	queuedMu.RLock()
	factory, ok := queuedFactory[j.Notification]
	queuedMu.RUnlock()
	if !ok {
		return fmt.Errorf("notifications: %q is not registered - call notifications.RegisterQueued before starting the worker", j.Notification)
	}
	notification := factory()
	if err := json.Unmarshal(j.Data, notification); err != nil {
		return fmt.Errorf("notifications: corrupt queued payload: %w", err)
	}

	for channel, address := range j.Routes {
		if err := manager.sendOn(channel, Route(channel, address), notification); err != nil {
			return fmt.Errorf("notifications: channel %s: %w", channel, err)
		}
	}
	return nil
}

// SendQueued dispatches the notification onto the queue instead of
// delivering inline. Channel routes are resolved now; delivery happens
// on a worker.
func (m *Manager) SendQueued(q queue.Queue, notifiable Notifiable, notification Notification) error {
	name := nameFor(notification)
	queuedMu.RLock()
	_, registered := queuedFactory[name]
	queuedMu.RUnlock()
	if !registered {
		return fmt.Errorf("notifications: %q is not registered for queueing - call notifications.RegisterQueued[%s]()",
			name, reflect.TypeOf(notification).Elem().Name())
	}

	routes := make(map[string]any)
	for _, channel := range notification.Via(notifiable) {
		if address := notifiable.RouteNotificationFor(channel); address != nil {
			routes[channel] = address
		}
	}
	if len(routes) == 0 {
		return nil // the notifiable routes none of the channels
	}

	data, err := json.Marshal(notification)
	if err != nil {
		return err
	}
	return q.Push(&queuedNotificationJob{
		Notification: name,
		Data:         data,
		Routes:       routes,
	})
}

func init() {
	queue.Register[queuedNotificationJob]()
}
