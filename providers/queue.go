package providers

import (
	"github.com/genesysflow/go-genesys/contracts"
	"github.com/genesysflow/go-genesys/queue"
)

// QueueServiceProvider registers the queue services.
type QueueServiceProvider struct {
	BaseProvider
}

// Register registers the queue services.
func (p *QueueServiceProvider) Register(app contracts.Application) error {
	p.app = app

	manager := queue.NewManager()
	app.InstanceType(manager)
	app.BindValue("queue", manager)

	return nil
}

// Boot bootstraps the queue services.
func (p *QueueServiceProvider) Boot(app contracts.Application) error {
	return nil
}

// Provides returns the services this provider registers.
func (p *QueueServiceProvider) Provides() []string {
	return []string{
		"queue",
	}
}
