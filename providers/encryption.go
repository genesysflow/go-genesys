package providers

import (
	"github.com/genesysflow/go-genesys/contracts"
	"github.com/genesysflow/go-genesys/crypt"
	"github.com/genesysflow/go-genesys/database"
	"github.com/genesysflow/go-genesys/env"
	facadecrypt "github.com/genesysflow/go-genesys/facades/crypt"
)

// EncryptionServiceProvider registers the application encrypter, keyed by
// APP_KEY (config app.key falls back to the environment variable).
type EncryptionServiceProvider struct {
	BaseProvider

	// Key optionally overrides the application key (mainly for tests).
	Key string
}

// Register registers the encryption services.
func (p *EncryptionServiceProvider) Register(app contracts.Application) error {
	p.app = app
	return nil
}

// Boot builds the encrypter once configuration is loaded.
func (p *EncryptionServiceProvider) Boot(app contracts.Application) error {
	key := p.Key
	if key == "" {
		key = app.GetConfig().GetString("app.key")
	}
	if key == "" {
		key = env.Get("APP_KEY")
	}
	if key == "" {
		// Leave the facade uninitialised; using it without a key panics
		// with instructions rather than failing every app at boot.
		return nil
	}

	encrypter, err := crypt.NewFromString(key)
	if err != nil {
		return err
	}
	app.InstanceType(encrypter)
	app.BindValue("encrypter", encrypter)
	facadecrypt.SetInstance(encrypter)
	database.SetEncrypter(encrypter) // powers `db:"...,encrypted"` casts
	return nil
}

// Provides returns the services this provider registers.
func (p *EncryptionServiceProvider) Provides() []string {
	return []string{
		"encrypter",
	}
}
