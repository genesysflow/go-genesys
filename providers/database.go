package providers

import (
	"fmt"
	"strconv"
	"time"

	"github.com/genesysflow/go-genesys/contracts"
	"github.com/genesysflow/go-genesys/database"
	"github.com/genesysflow/go-genesys/facades/db"

	_ "modernc.org/sqlite"
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
						if v, ok := connDetails["connect_retry"]; ok {
							retry, err := parseConnectRetry(v)
							if err != nil {
								return fmt.Errorf("database connection [%s]: connect_retry: %w", name, err)
							}
							connConfig.ConnectRetry = retry
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
	app.Instance("db", manager)
	app.Instance("database", manager)
	app.InstanceType(manager) // Registers as *database.Manager

	// Initialize the DB facade and the package-level ORM helpers
	db.SetInstance(manager)
	database.SetDefault(manager)

	return nil
}

// parseConnectRetry reads a connect_retry value: a duration string
// ("15s", "500ms"), or a number of seconds (15, or "15" after env
// interpolation). A negative value disables retries.
func parseConnectRetry(v any) (time.Duration, error) {
	switch v := v.(type) {
	case nil:
		return 0, nil
	case int:
		return time.Duration(v) * time.Second, nil
	case int64:
		return time.Duration(v) * time.Second, nil
	case float64:
		return time.Duration(v * float64(time.Second)), nil
	case string:
		if v == "" {
			return 0, nil
		}
		if n, err := strconv.Atoi(v); err == nil {
			return time.Duration(n) * time.Second, nil
		}
		d, err := time.ParseDuration(v)
		if err != nil {
			return 0, fmt.Errorf("want a duration like \"15s\" or a number of seconds, got %q", v)
		}
		return d, nil
	default:
		return 0, fmt.Errorf("want a duration like \"15s\" or a number of seconds, got %T", v)
	}
}

// Provides returns the services this provider registers.
func (p *DatabaseServiceProvider) Provides() []string {
	return []string{
		"db",
		"database",
		"db.manager",
	}
}
