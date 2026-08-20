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

// typeKey identifies a notification type unambiguously (package path +
// name), so same-named types in different packages cannot collide in
// the registry. The stored display name (nameFor) stays snake-cased.
func typeKey(notification Notification) string {
	t := reflect.TypeOf(notification)
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.PkgPath() != "" {
		return t.PkgPath() + "." + t.Name()
	}
	return t.Name()
}

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
	queuedMu.Lock()
	defer queuedMu.Unlock()
	queuedFactory[typeKey(notification)] = func() Notification {
		var instance N
		return any(&instance).(Notification)
	}
}

// queuedNotificationJob is the serializable envelope for one channel
// of one notifiable. One job per channel means a transient failure on
// one channel retries only that channel - a succeeded email is never
// re-sent because the database insert flaked.
type queuedNotificationJob struct {
	Notification string          `json:"notification"`
	Data         json.RawMessage `json:"data"`
	Channel      string          `json:"channel"`
	// Route is the resolved address, normalized to a string at dispatch
	// so numeric ids survive the JSON round-trip exactly (float64 would
	// render 1000000 as "1e+06").
	Route string `json:"route"`
}

// JobName gives the wrapper a stable registered name.
func (j *queuedNotificationJob) JobName() string { return "genesys.notification" }

// Handle reconstructs the notification and delivers it on the job's
// channel through the default manager.
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

	if err := manager.sendOn(j.Channel, Route(j.Channel, j.Route), notification); err != nil {
		return fmt.Errorf("notifications: channel %s: %w", j.Channel, err)
	}
	return nil
}

// SendQueued dispatches the notification onto the queue instead of
// delivering inline. Channel routes are resolved now; delivery happens
// on a worker.
func (m *Manager) SendQueued(q queue.Queue, notifiable Notifiable, notification Notification) error {
	key := typeKey(notification)
	queuedMu.RLock()
	_, registered := queuedFactory[key]
	queuedMu.RUnlock()
	if !registered {
		return fmt.Errorf("notifications: %s is not registered for queueing - call notifications.RegisterQueued for it first", key)
	}

	data, err := json.Marshal(notification)
	if err != nil {
		return err
	}
	for _, channel := range notification.Via(notifiable) {
		address := notifiable.RouteNotificationFor(channel)
		if address == nil {
			continue // the notifiable opted out of this channel
		}
		if err := q.Push(&queuedNotificationJob{
			Notification: key,
			Data:         data,
			Channel:      channel,
			Route:        fmt.Sprint(address),
		}); err != nil {
			return err
		}
	}
	return nil
}

func init() {
	queue.Register[queuedNotificationJob]()
}
