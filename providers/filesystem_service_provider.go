package providers

import (
	"github.com/genesysflow/go-genesys/contracts"
	"github.com/genesysflow/go-genesys/facades/storage"
	"github.com/genesysflow/go-genesys/filesystem"
)

// FilesystemServiceProvider registers filesystem services.
type FilesystemServiceProvider struct{}

// Register registers the filesystem services.
func (p *FilesystemServiceProvider) Register(app contracts.Application) error {
	// Register the filesystem manager
	app.Singleton("filesystem", func(app contracts.Application) (contracts.FilesystemFactory, error) {
		return filesystem.NewManager(app.GetConfig()), nil
	})

	// Bind the filesystem manager interface
	app.SingletonType(func(app contracts.Application) (contracts.FilesystemFactory, error) {
		return app.MustMake("filesystem").(contracts.FilesystemFactory), nil
	})

	// Register the default disk driver
	app.Singleton("filesystem.disk", func(app contracts.Application) (contracts.Filesystem, error) {
		manager := app.MustMake("filesystem").(contracts.FilesystemFactory)
		return manager.Disk(), nil
	})

	// Bind the filesystem contract to standard default disk
	app.SingletonType(func(app contracts.Application) (contracts.Filesystem, error) {
		return app.MustMake("filesystem.disk").(contracts.Filesystem), nil
	})

	return nil
}

// Boot bootstraps the filesystem services.
func (p *FilesystemServiceProvider) Boot(app contracts.Application) error {
	storage.SetInstance(app.MustMake("filesystem").(contracts.FilesystemFactory))
	return nil
}

// Provides returns the services this provider registers.
func (p *FilesystemServiceProvider) Provides() []string {
	return []string{
		"filesystem",
		"filesystem.disk",
		"contracts.FilesystemFactory",
		"contracts.Filesystem",
	}
}
