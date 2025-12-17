package providers

import (
	"github.com/genesysflow/go-genesys/contracts"
	"github.com/genesysflow/go-genesys/events"
)

// EventServiceProvider registers the event dispatcher.
type EventServiceProvider struct {
	BaseProvider
}

// Register registers the event services.
func (p *EventServiceProvider) Register(app contracts.Application) error {
	p.app = app

	dispatcher := events.NewDispatcher()
	app.InstanceType(dispatcher)
	app.BindValue("events", dispatcher)

	return nil
}

// Boot bootstraps the event services.
func (p *EventServiceProvider) Boot(app contracts.Application) error {
	return nil
}

// Provides returns the services this provider registers.
func (p *EventServiceProvider) Provides() []string {
	return []string{
		"events",
	}
}
