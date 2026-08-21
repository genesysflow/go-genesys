package providers

import (
	"github.com/genesysflow/go-genesys/cache"
	"github.com/genesysflow/go-genesys/container"
	"github.com/genesysflow/go-genesys/contracts"
	"github.com/genesysflow/go-genesys/schedule"
)

// ScheduleServiceProvider registers the task scheduler.
//
//	app.Register(&providers.ScheduleServiceProvider{
//	    Define: func(s *schedule.Schedule) {
//	        s.Call(pruneSessions).Daily().Description("prune sessions")
//	    },
//	})
type ScheduleServiceProvider struct {
	BaseProvider

	// Define registers the application's scheduled tasks.
	Define func(*schedule.Schedule)
}

// Register registers the scheduler.
func (p *ScheduleServiceProvider) Register(app contracts.Application) error {
	p.app = app

	scheduler := schedule.New()

	// Events constrained with Environments() need to know where they are
	// running, and OnOneServer() needs a store to coordinate through.
	scheduler.SetEnvironment(app.Environment())
	if manager, err := container.Resolve[*cache.Manager](app); err == nil {
		if store, err := manager.Store(); err == nil {
			scheduler.UseCache(store)
		}
	}

	if p.Define != nil {
		p.Define(scheduler)
	}
	app.InstanceType(scheduler)
	app.BindValue("schedule", scheduler)

	return nil
}

// Boot bootstraps the scheduler.
func (p *ScheduleServiceProvider) Boot(app contracts.Application) error {
	return nil
}

// Provides returns the services this provider registers.
func (p *ScheduleServiceProvider) Provides() []string {
	return []string{
		"schedule",
	}
}
