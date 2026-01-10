package providers

import (
	"github.com/genesysflow/go-genesys/contracts"
)

// AppServiceProvider registers application services.
type AppServiceProvider struct{}

func (p *AppServiceProvider) Register(app contracts.Application) error {
	// Register application services here
	// With SQLC, use your generated queries like:
	//   db.New(connection.DB()) to get type-safe queries
	return nil
}

func (p *AppServiceProvider) Boot(app contracts.Application) error {
	// Boot logic
	return nil
}

func (p *AppServiceProvider) Provides() []string {
	return []string{}
}
