package providers

import (
	"fmt"

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
		service := app.MustMake("filesystem")
		manager, ok := service.(contracts.FilesystemFactory)
		if !ok {
			return nil, fmt.Errorf("filesystem service is not of type contracts.FilesystemFactory")
		}
		return manager, nil
	})

	// Register the default disk driver
	app.Singleton("filesystem.disk", func(app contracts.Application) (contracts.Filesystem, error) {
		service := app.MustMake("filesystem")
		manager, ok := service.(contracts.FilesystemFactory)
		if !ok {
			return nil, fmt.Errorf("filesystem service is not of type contracts.FilesystemFactory")
		}
		return manager.Disk(), nil
	})

	// Bind the filesystem contract to standard default disk
	app.SingletonType(func(app contracts.Application) (contracts.Filesystem, error) {
		service := app.MustMake("filesystem.disk")
		fs, ok := service.(contracts.Filesystem)
		if !ok {
			return nil, fmt.Errorf("filesystem.disk service is not of type contracts.Filesystem")
		}
		return fs, nil
	})

	return nil
}

// Boot bootstraps the filesystem services.
func (p *FilesystemServiceProvider) Boot(app contracts.Application) error {
	service := app.MustMake("filesystem")
	manager, ok := service.(contracts.FilesystemFactory)
	if !ok {
		return fmt.Errorf("filesystem service is not of type contracts.FilesystemFactory")
	}
	storage.SetInstance(manager)
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
