// Package tasks holds the example application's scheduled work.
package tasks

import (
	"time"

	"github.com/genesysflow/go-genesys/container"
	"github.com/genesysflow/go-genesys/contracts"
	"github.com/genesysflow/go-genesys/database"
	"github.com/genesysflow/go-genesys/example/app/jobs"
	"github.com/genesysflow/go-genesys/example/app/models"
	"github.com/genesysflow/go-genesys/queue"
)

// staleAfter is how long a draft may sit untouched before it is pruned.
const staleAfter = 180 * 24 * time.Hour

// PruneStaleDrafts soft-deletes drafts nobody has touched in months.
// Soft, not hard: an author who comes back should find their work, not a
// hole where it was.
func PruneStaleDrafts() error {
	cutoff := time.Now().Add(-staleAfter)

	_, err := database.Query[models.Post]().
		WhereNull("published_at").
		Where("updated_at", "<", cutoff).
		Delete()

	return err
}

// QueueWeeklyDigest returns a task that pushes the digest job onto the
// queue, so the scheduler process hands the work off rather than doing
// it itself.
func QueueWeeklyDigest(app contracts.Application) func() error {
	return func() error {
		manager, err := container.Resolve[*queue.Manager](app)
		if err != nil {
			return err
		}

		connection, err := manager.Connection()
		if err != nil {
			return err
		}

		return connection.Push(&jobs.WeeklyDigest{Since: time.Now().Add(-7 * 24 * time.Hour)})
	}
}
