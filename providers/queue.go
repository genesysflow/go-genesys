package providers

import (
	"fmt"

	"github.com/genesysflow/go-genesys/container"
	"github.com/genesysflow/go-genesys/contracts"
	"github.com/genesysflow/go-genesys/database"
	facadequeue "github.com/genesysflow/go-genesys/facades/queue"
	"github.com/genesysflow/go-genesys/queue"
)

// QueueServiceProvider registers the queue services.
//
// Configuration (config/queue.yaml):
//
//	default: sync
//	connections:
//	  sync:
//	    driver: sync
//	  database:
//	    driver: database
//	    table: jobs
//	    failed_table: failed_jobs
//	    queue: default
type QueueServiceProvider struct {
	BaseProvider

	manager *queue.Manager
}

// Register registers the queue services.
func (p *QueueServiceProvider) Register(app contracts.Application) error {
	p.app = app

	manager := queue.NewManager()
	p.manager = manager
	app.InstanceType(manager)
	app.BindValue("queue", manager)
	facadequeue.SetInstance(manager)

	return nil
}

// Boot wires configured queue connections.
func (p *QueueServiceProvider) Boot(app contracts.Application) error {
	cfg := app.GetConfig()

	if def := cfg.GetString("queue.default"); def != "" {
		p.manager.SetDefaultConnection(def)
	}

	connections, ok := cfg.Get("queue.connections").(map[string]any)
	if !ok {
		return nil
	}

	for name, raw := range connections {
		details, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		driver, _ := details["driver"].(string)

		switch driver {
		case "sync":
			p.manager.Register(name, queue.NewSyncQueue())
		case "memory":
			p.manager.Register(name, queue.NewMemoryQueue())
		case "database":
			dbCfg := queue.DatabaseQueueConfig{}
			if table, ok := details["table"].(string); ok {
				dbCfg.Table = table
			}
			if failed, ok := details["failed_table"].(string); ok {
				dbCfg.FailedTable = failed
			}
			if queueName, ok := details["queue"].(string); ok {
				dbCfg.Queue = queueName
			}
			dbConnection, _ := details["connection"].(string)

			p.manager.RegisterLazy(name, func() (queue.Queue, error) {
				dbManager, err := container.Resolve[*database.Manager](app)
				if err != nil {
					return nil, fmt.Errorf("queue: database driver requires the DatabaseServiceProvider: %w", err)
				}
				conn := dbManager.Connection(dbConnection)
				if conn == nil {
					return nil, fmt.Errorf("queue: database connection %q not available", dbConnection)
				}
				return queue.NewDatabaseQueue(conn.Driver(), conn, dbCfg), nil
			})
		}
	}

	return nil
}

// Provides returns the services this provider registers.
func (p *QueueServiceProvider) Provides() []string {
	return []string{
		"queue",
	}
}
