package providers

import (
	"github.com/genesysflow/go-genesys/contracts"
	"github.com/genesysflow/go-genesys/example/app/controllers"
	"github.com/genesysflow/go-genesys/example/app/services"
)

type AppServiceProvider struct{}

func (p *AppServiceProvider) Register(app contracts.Application) error {
	// Register UserService
	app.SingletonType(services.NewUserService)

	// Register UserController
	// The container will automatically inject UserService because NewUserController will ask for it
	app.BindType(controllers.NewUserController)
	return nil
}

func (p *AppServiceProvider) Boot(app contracts.Application) error {
	// Boot logic
	return nil
}

func (p *AppServiceProvider) Provides() []string {
	return []string{}
}
