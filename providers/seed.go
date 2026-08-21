package providers

import (
	"github.com/genesysflow/go-genesys/contracts"
	"github.com/genesysflow/go-genesys/database/seed"
)

// SeedServiceProvider registers the database seeder runner.
//
//	app.Register(&providers.SeedServiceProvider{
//	    Define: func(r *seed.Runner) {
//	        r.AddFunc("users", seeders.SeedUsers)
//	    },
//	})
type SeedServiceProvider struct {
	BaseProvider

	// Define registers the application's seeders.
	Define func(*seed.Runner)
}

// Register registers the seeder runner.
func (p *SeedServiceProvider) Register(app contracts.Application) error {
	p.app = app

	runner := seed.NewRunner()
	if p.Define != nil {
		p.Define(runner)
	}
	app.InstanceType(runner)
	app.BindValue("seeder", runner)

	return nil
}

// Boot bootstraps the seeder services.
func (p *SeedServiceProvider) Boot(app contracts.Application) error {
	return nil
}

// Provides returns the services this provider registers.
func (p *SeedServiceProvider) Provides() []string {
	return []string{
		"seeder",
	}
}
