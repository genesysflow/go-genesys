package providers

import (
	"github.com/genesysflow/go-genesys/container"
	"github.com/genesysflow/go-genesys/contracts"
	"github.com/genesysflow/go-genesys/database"
	facadenotify "github.com/genesysflow/go-genesys/facades/notify"
	"github.com/genesysflow/go-genesys/mail"
	"github.com/genesysflow/go-genesys/notifications"
)

// NotificationServiceProvider wires the notification manager with the
// registered mailer and (when available) the default database
// connection for the database channel.
//
// The database channel expects a notifications table:
//
//	id INTEGER PRIMARY KEY, notifiable_id TEXT, name TEXT,
//	data TEXT, read_at TIMESTAMP NULL, created_at TIMESTAMP
type NotificationServiceProvider struct {
	BaseProvider

	// Table overrides the database channel's table name.
	Table string
}

// Register binds the notification manager into the container.
func (p *NotificationServiceProvider) Register(app contracts.Application) error {
	return nil
}

// Boot assembles the manager from whatever channels are available.
func (p *NotificationServiceProvider) Boot(app contracts.Application) error {
	var options []notifications.Option

	if mailer, err := container.Resolve[mail.Mailer](app, "mailer"); err == nil && mailer != nil {
		options = append(options, notifications.WithMailer(mailer))
	}
	if dbManager, err := container.Resolve[*database.Manager](app); err == nil && dbManager != nil {
		if conn := dbManager.Connection(); conn.Error() == nil {
			options = append(options, notifications.WithDatabase(conn.Driver(), conn, p.Table))
		}
	}

	manager := notifications.New(options...)
	app.InstanceType(manager)
	app.BindValue("notifications", manager)
	facadenotify.SetInstance(manager)
	notifications.SetDefault(manager) // queued notifications deliver through this
	return nil
}

// Provides returns the services this provider registers.
func (p *NotificationServiceProvider) Provides() []string {
	return []string{"notifications"}
}
