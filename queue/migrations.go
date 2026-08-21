package queue

import (
	"github.com/genesysflow/go-genesys/database/schema"
)

// CreateJobsTables creates the jobs and failed_jobs tables used by the
// database queue driver. Call it from an application migration:
//
//	func (m *CreateQueueTables) Up(builder *schema.Builder) error {
//	    return queue.CreateJobsTables(builder)
//	}
func CreateJobsTables(builder *schema.Builder) error {
	if err := builder.Create("jobs", func(table *schema.Blueprint) {
		table.BigIncrements("id")
		table.String("queue", 255)
		table.String("name", 255)
		table.Text("payload")
		table.Integer("attempts")
		table.BigInteger("reserved_at").Nullable()
		table.BigInteger("available_at")
		table.BigInteger("created_at")
		table.Index("queue")
	}); err != nil {
		return err
	}

	return builder.Create("failed_jobs", func(table *schema.Blueprint) {
		table.BigIncrements("id")
		table.String("queue", 255)
		table.String("name", 255)
		table.Text("payload")
		table.Text("exception")
		table.BigInteger("failed_at")
	})
}

// DropJobsTables drops the queue tables.
func DropJobsTables(builder *schema.Builder) error {
	if err := builder.DropIfExists("failed_jobs"); err != nil {
		return err
	}
	return builder.DropIfExists("jobs")
}
