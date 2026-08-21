// Package notifications delivers notifications through multiple
// channels, Laravel's notification system. A notification declares its
// channels with Via and provides per-channel content through the
// optional ToMail/ToDatabase methods:
//
//	type InvoicePaid struct{ Amount int }
//
//	func (n *InvoicePaid) Via(_ notifications.Notifiable) []string {
//	    return []string{"mail", "database"}
//	}
//	func (n *InvoicePaid) ToMail(_ notifications.Notifiable) *mail.Message {
//	    return mail.NewMessage().Subject("Invoice paid").Text("Thanks!")
//	}
//	func (n *InvoicePaid) ToDatabase(_ notifications.Notifiable) map[string]any {
//	    return map[string]any{"amount": n.Amount}
//	}
//
//	manager.Send(user, &InvoicePaid{Amount: 100})
package notifications

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"sync"
	"time"

	"github.com/genesysflow/go-genesys/mail"
	"github.com/genesysflow/go-genesys/query"
	"github.com/genesysflow/go-genesys/support"
)

// Notifiable is something that can receive notifications. Models
// typically route "mail" to their email column and "database" to their
// primary key.
type Notifiable interface {
	// RouteNotificationFor returns the address for a channel: an email
	// address for "mail", an identifier for "database". Returning nil
	// skips the channel.
	RouteNotificationFor(channel string) any
}

// Notification declares which channels a notification uses.
type Notification interface {
	Via(notifiable Notifiable) []string
}

// MailNotification provides the mail channel's message.
type MailNotification interface {
	ToMail(notifiable Notifiable) *mail.Message
}

// DatabaseNotification provides the database channel's payload.
type DatabaseNotification interface {
	ToDatabase(notifiable Notifiable) map[string]any
}

// Nameable overrides the stored notification name (defaults to the
// snake_cased type name).
type Nameable interface {
	NotificationName() string
}

// route is an on-demand notifiable for one-off addresses.
type route struct {
	channel string
	address any
}

func (r *route) RouteNotificationFor(channel string) any {
	if channel == r.channel {
		return r.address
	}
	return nil
}

// Route creates an on-demand notifiable for a single channel address:
//
//	manager.Send(notifications.Route("mail", "ops@example.com"), &ServerDown{})
func Route(channel string, address any) Notifiable {
	return &route{channel: channel, address: address}
}

// Manager sends notifications through its configured channels.
type Manager struct {
	mailer mail.Mailer

	// database channel dependencies (optional)
	driver   string
	executor query.Executor
	table    string

	// broadcast and webhook channel dependencies (optional)
	broadcaster Broadcaster
	httpClient  *http.Client

	// channels registered with Extend
	custom map[string]ChannelFunc
	mu     sync.RWMutex
}

// Option configures a Manager.
type Option func(*Manager)

// WithMailer enables the mail channel.
func WithMailer(mailer mail.Mailer) Option {
	return func(m *Manager) { m.mailer = mailer }
}

// WithDatabase enables the database channel. The table needs columns
// id, notifiable_id, name, data, read_at, created_at.
func WithDatabase(driver string, executor query.Executor, table string) Option {
	return func(m *Manager) {
		m.driver = driver
		m.executor = executor
		if table == "" {
			table = "notifications"
		}
		m.table = table
	}
}

// New creates a notification manager.
func New(options ...Option) *Manager {
	m := &Manager{}
	for _, option := range options {
		option(m)
	}
	return m
}

// Send delivers the notification on every channel it declares.
func (m *Manager) Send(notifiable Notifiable, notification Notification) error {
	for _, channel := range notification.Via(notifiable) {
		if err := m.sendOn(channel, notifiable, notification); err != nil {
			return fmt.Errorf("notifications: channel %s: %w", channel, err)
		}
	}
	return nil
}

func (m *Manager) sendOn(channel string, notifiable Notifiable, notification Notification) error {
	switch channel {
	case "mail":
		return m.sendMail(notifiable, notification)
	case "database":
		return m.sendDatabase(notifiable, notification)
	case "broadcast":
		return m.sendBroadcast(notifiable, notification)
	case "webhook":
		return m.sendWebhook(notifiable, notification)
	default:
		if fn, ok := m.customChannel(channel); ok {
			return fn(notifiable, notification)
		}
		// A channel nobody handles means a notification nobody receives,
		// which is worse than one that fails loudly.
		return fmt.Errorf("unknown channel (built in: mail, database, broadcast, webhook; register others with Extend)")
	}
}

func (m *Manager) sendMail(notifiable Notifiable, notification Notification) error {
	if m.mailer == nil {
		return fmt.Errorf("no mailer configured (use notifications.WithMailer)")
	}
	mailable, ok := notification.(MailNotification)
	if !ok {
		return fmt.Errorf("%T does not implement ToMail", notification)
	}
	address := notifiable.RouteNotificationFor("mail")
	if address == nil {
		return nil // the notifiable opted out of this channel
	}
	email, ok := address.(string)
	if !ok || email == "" {
		return fmt.Errorf("mail route must be a non-empty string, got %T", address)
	}
	message := mailable.ToMail(notifiable)
	if message == nil {
		return nil
	}
	if len(message.Recipients()) == 0 {
		message.To(email)
	}
	return m.mailer.Send(message)
}

func (m *Manager) sendDatabase(notifiable Notifiable, notification Notification) error {
	if m.executor == nil {
		return fmt.Errorf("no database configured (use notifications.WithDatabase)")
	}
	storable, ok := notification.(DatabaseNotification)
	if !ok {
		return fmt.Errorf("%T does not implement ToDatabase", notification)
	}
	id := notifiable.RouteNotificationFor("database")
	if id == nil {
		return nil
	}
	data, err := json.Marshal(storable.ToDatabase(notifiable))
	if err != nil {
		return err
	}
	return query.New(m.driver, m.executor).Table(m.table).Insert(map[string]any{
		"notifiable_id": fmt.Sprint(id),
		"name":          nameFor(notification),
		"data":          string(data),
		"read_at":       nil,
		"created_at":    time.Now().UTC(),
	})
}

// nameFor resolves the stored notification name.
func nameFor(notification Notification) string {
	if nameable, ok := notification.(Nameable); ok {
		return nameable.NotificationName()
	}
	t := reflect.TypeOf(notification)
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return support.ToSnakeCase(t.Name())
}

// Stored is a database notification row.
type Stored struct {
	ID           int64          `json:"id"`
	NotifiableID string         `json:"notifiable_id"`
	Name         string         `json:"name"`
	Data         map[string]any `json:"data"`
	ReadAt       *time.Time     `json:"read_at"`
	CreatedAt    time.Time      `json:"created_at"`
}

// For returns all database notifications for a notifiable, newest first.
func (m *Manager) For(notifiableID any) ([]Stored, error) {
	return m.list(notifiableID, false)
}

// UnreadFor returns unread database notifications, newest first.
func (m *Manager) UnreadFor(notifiableID any) ([]Stored, error) {
	return m.list(notifiableID, true)
}

func (m *Manager) list(notifiableID any, unreadOnly bool) ([]Stored, error) {
	if m.executor == nil {
		return nil, fmt.Errorf("notifications: no database configured")
	}
	builder := query.New(m.driver, m.executor).Table(m.table).
		Where("notifiable_id", fmt.Sprint(notifiableID)).
		OrderByDesc("created_at").OrderByDesc("id")
	if unreadOnly {
		builder.WhereNull("read_at")
	}
	rows, err := builder.Get()
	if err != nil {
		return nil, err
	}

	stored := make([]Stored, 0, len(rows))
	for _, row := range rows {
		item := Stored{
			NotifiableID: fmt.Sprint(row["notifiable_id"]),
			Name:         fmt.Sprint(row["name"]),
		}
		if id, ok := row["id"].(int64); ok {
			item.ID = id
		}
		if raw, ok := row["data"].(string); ok {
			json.Unmarshal([]byte(raw), &item.Data)
		}
		if raw, ok := row["data"].([]byte); ok {
			json.Unmarshal(raw, &item.Data)
		}
		if created, ok := scanTime(row["created_at"]); ok {
			item.CreatedAt = created
		}
		if read, ok := scanTime(row["read_at"]); ok {
			item.ReadAt = &read
		}
		stored = append(stored, item)
	}
	return stored, nil
}

// scanTime converts a driver-returned timestamp column: postgres hands
// back time.Time, sqlite a string, mysql []byte.
func scanTime(v any) (time.Time, bool) {
	switch t := v.(type) {
	case time.Time:
		return t, true
	case string:
		return parseTimeString(t)
	case []byte:
		return parseTimeString(string(t))
	}
	return time.Time{}, false
}

func parseTimeString(s string) (time.Time, bool) {
	for _, layout := range []string{
		time.RFC3339Nano,
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// MarkAsRead stamps a database notification as read.
func (m *Manager) MarkAsRead(id int64) error {
	if m.executor == nil {
		return fmt.Errorf("notifications: no database configured")
	}
	_, err := query.New(m.driver, m.executor).Table(m.table).
		Where("id", id).Update(map[string]any{"read_at": time.Now().UTC()})
	return err
}

// MarkAllAsRead stamps all of a notifiable's notifications as read.
func (m *Manager) MarkAllAsRead(notifiableID any) error {
	if m.executor == nil {
		return fmt.Errorf("notifications: no database configured")
	}
	_, err := query.New(m.driver, m.executor).Table(m.table).
		Where("notifiable_id", fmt.Sprint(notifiableID)).
		WhereNull("read_at").
		Update(map[string]any{"read_at": time.Now().UTC()})
	return err
}
