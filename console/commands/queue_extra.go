package commands

import (
	"fmt"
	"strings"

	"github.com/genesysflow/go-genesys/cache"
	"github.com/genesysflow/go-genesys/console/cli"
	"github.com/genesysflow/go-genesys/container"
	"github.com/genesysflow/go-genesys/contracts"
	"github.com/genesysflow/go-genesys/queue"
)

// QueueMonitorCommand reports how many jobs are waiting on each queue,
// Laravel's `artisan queue:monitor`.
func QueueMonitorCommand(app contracts.Application) *cli.Command {
	return &cli.Command{
		Name:        "queue:monitor",
		Description: "Report the size of one or more queues",
		Arguments: []cli.Argument{
			{Name: "queues", Description: "Comma-separated queue names", Default: "default"},
		},
		Options: []cli.Option{
			{Name: "max", Description: "Report a backlog above this size", Default: "0"},
			{Name: "connection", Description: "The queue connection to inspect", Default: "default"},
		},
		Handle: func(c *cli.Context) error {
			// "default" names the configured default connection rather
			// than a connection literally called that.
			connection := c.Option("connection")
			if connection == "default" {
				connection = ""
			}

			driver, err := resolveQueueConnection(c.App(), connection)
			if err != nil {
				return err
			}

			sizer, ok := driver.(queue.SizeProvider)
			if !ok {
				// Reporting zero for a driver that cannot count would
				// read as "nothing is waiting", which is the opposite of
				// what a monitor is for.
				return fmt.Errorf("queue:monitor: the %T driver cannot report queue sizes", driver)
			}

			max := c.IntOption("max")
			rows := make([][]string, 0)

			for _, name := range splitQueueNames(c.Argument("queues")) {
				size, err := sizer.Size(name)
				if err != nil {
					return fmt.Errorf("queue:monitor: reading %s: %w", name, err)
				}

				status := "ok"
				if max > 0 && size > int64(max) {
					status = fmt.Sprintf("above %d", max)
				}
				rows = append(rows, []string{name, fmt.Sprint(size), status})
			}

			c.Table([]string{"Queue", "Jobs", "Status"}, rows)
			return nil
		},
	}
}

// QueueRestartCommand asks running workers to stop once they finish
// their current job, Laravel's `artisan queue:restart`.
//
// The workers' supervisor starts them again, which is how a deploy gets
// new code onto the queue without killing a job mid-flight. Workers
// honour it only when they call Worker.WatchRestart.
func QueueRestartCommand(app contracts.Application) *cli.Command {
	return &cli.Command{
		Name:        "queue:restart",
		Description: "Ask running queue workers to restart after their current job",
		Handle: func(c *cli.Context) error {
			manager, err := container.Resolve[*cache.Manager](c.App())
			if err != nil {
				return fmt.Errorf("queue:restart: a cache store is where the signal is left, and none is configured: %w", err)
			}

			store, err := manager.Store()
			if err != nil {
				return fmt.Errorf("queue:restart: %w", err)
			}

			if err := queue.SignalRestart(store); err != nil {
				return fmt.Errorf("queue:restart: %w", err)
			}

			c.Info("Broadcasting a restart signal to the workers.")
			return nil
		},
	}
}

// splitQueueNames parses a comma-separated queue list.
func splitQueueNames(value string) []string {
	names := make([]string, 0)
	for _, name := range strings.Split(value, ",") {
		if trimmed := strings.TrimSpace(name); trimmed != "" {
			names = append(names, trimmed)
		}
	}
	if len(names) == 0 {
		return []string{"default"}
	}
	return names
}
