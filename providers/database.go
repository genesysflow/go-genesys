package providers

import (
	"github.com/genesysflow/go-genesys/contracts"
	"github.com/genesysflow/go-genesys/database"
	"github.com/genesysflow/go-genesys/facades/db"
)

// DatabaseServiceProvider registers database services.
type DatabaseServiceProvider struct {
	BaseProvider

	// Config is optional database configuration.
	// If nil, configuration is loaded from config/database.yaml
	Config *database.Config
}

// Register registers the database services.
func (p *DatabaseServiceProvider) Register(app contracts.Application) error {
	p.app = app
	return nil
}

// Boot bootstraps the database services.
func (p *DatabaseServiceProvider) Boot(app contracts.Application) error {
	cfg := app.GetConfig()

	// Build database config from app config or use provided config
	dbConfig := database.Config{
		Default:     "default",
		Connections: make(map[string]database.ConnectionConfig),
	}

	if p.Config != nil {
		dbConfig = *p.Config
	} else {
		// Load from config file
		if defaultConn := cfg.GetString("database.default"); defaultConn != "" {
			dbConfig.Default = defaultConn
		}

		// Load connections from config
		if connections := cfg.Get("database.connections"); connections != nil {
			if connMap, ok := connections.(map[string]any); ok {
				for name, connCfg := range connMap {
					if connDetails, ok := connCfg.(map[string]any); ok {
						connConfig := database.ConnectionConfig{}

						if driver, ok := connDetails["driver"].(string); ok {
							connConfig.Driver = driver
						}
						if host, ok := connDetails["host"].(string); ok {
							connConfig.Host = host
						}
						if port, ok := connDetails["port"].(int); ok {
							connConfig.Port = port
						}
						if dbName, ok := connDetails["database"].(string); ok {
							connConfig.Database = dbName
						}
						if username, ok := connDetails["username"].(string); ok {
							connConfig.Username = username
						}
						if password, ok := connDetails["password"].(string); ok {
							connConfig.Password = password
						}
						if sslmode, ok := connDetails["sslmode"].(string); ok {
							connConfig.SSLMode = sslmode
						}
						if prefix, ok := connDetails["prefix"].(string); ok {
							connConfig.Prefix = prefix
						}
						if fk, ok := connDetails["foreign_key_constraints"].(bool); ok {
							connConfig.ForeignKeyConstraints = fk
						}
						if maxOpen, ok := connDetails["max_open_conns"].(int); ok {
							connConfig.MaxOpenConns = maxOpen
						}
						if maxIdle, ok := connDetails["max_idle_conns"].(int); ok {
							connConfig.MaxIdleConns = maxIdle
						}

						dbConfig.Connections[name] = connConfig
					}
				}
			}
		}
	}

	// Create the database manager
	manager := database.NewManager(dbConfig)

	// Bind to container
	app.BindValue("db", manager)
	app.BindValue("database", manager)
	app.BindValue("db.manager", manager)

	// Initialize the DB facade
	db.SetInstance(manager)

	return nil
}

// Provides returns the services this provider registers.
func (p *DatabaseServiceProvider) Provides() []string {
	return []string{
		"db",
		"database",
		"db.manager",
	}
}
