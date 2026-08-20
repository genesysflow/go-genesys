package providers

import (
	"github.com/genesysflow/go-genesys/container"
	"github.com/genesysflow/go-genesys/contracts"
	facademail "github.com/genesysflow/go-genesys/facades/mail"
	"github.com/genesysflow/go-genesys/mail"
)

// MailServiceProvider registers the mailer.
//
// Configuration (config/mail.yaml):
//
//	driver: log            # smtp, log, array
//	host: smtp.example.com
//	port: 587
//	username: ${MAIL_USERNAME:}
//	password: ${MAIL_PASSWORD:}
//	encryption: starttls   # tls, starttls, none
//	from_address: hello@example.com
//	from_name: My App
type MailServiceProvider struct {
	BaseProvider

	// Config is optional mail configuration.
	Config *mail.Config
}

// Register registers the mail services.
func (p *MailServiceProvider) Register(app contracts.Application) error {
	p.app = app
	return nil
}

// Boot builds the configured mailer.
func (p *MailServiceProvider) Boot(app contracts.Application) error {
	cfg := mail.Config{}
	if p.Config != nil {
		cfg = *p.Config
	} else {
		appCfg := app.GetConfig()
		cfg.Driver = appCfg.GetString("mail.driver")
		cfg.Host = appCfg.GetString("mail.host")
		cfg.Port = appCfg.GetInt("mail.port")
		cfg.Username = appCfg.GetString("mail.username")
		cfg.Password = appCfg.GetString("mail.password")
		cfg.Encryption = appCfg.GetString("mail.encryption")
		cfg.FromAddress = appCfg.GetString("mail.from_address")
		cfg.FromName = appCfg.GetString("mail.from_name")
	}

	var mailer mail.Mailer
	switch cfg.Driver {
	case "smtp":
		mailer = mail.NewSMTPMailer(cfg)
	case "array":
		mailer = mail.NewArrayMailer(cfg)
	default:
		// Log driver is the safe default: no mail leaves the machine.
		logger, _ := container.Resolve[contracts.Logger](app)
		mailer = mail.NewLogMailer(cfg, logger)
	}

	app.BindValue("mailer", mailer)
	facademail.SetInstance(mailer)

	return nil
}

// Provides returns the services this provider registers.
func (p *MailServiceProvider) Provides() []string {
	return []string{
		"mailer",
	}
}
