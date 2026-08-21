package commands

import (
	"fmt"
	"time"

	"github.com/genesysflow/go-genesys/contracts"
	"github.com/genesysflow/go-genesys/queue"
	"github.com/spf13/cobra"
)

// QueuePruneFailedCommand creates the queue:prune-failed command,
// deleting failed jobs older than --hours (default 24).
func QueuePruneFailedCommand(app contracts.Application) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "queue:prune-failed",
		Short: "Delete failed queue jobs older than the retention window",
		RunE: func(cmd *cobra.Command, args []string) error {
			connName, _ := cmd.Flags().GetString("connection")
			hours, _ := cmd.Flags().GetInt("hours")

			conn, err := resolveQueueConnection(app, connName)
			if err != nil {
				return err
			}
			pruner, ok := conn.(queue.FailedJobPruner)
			if !ok {
				return fmt.Errorf("queue connection does not support pruning failed jobs")
			}

			pruned, err := pruner.PruneFailed(time.Now().Add(-time.Duration(hours) * time.Hour))
			if err != nil {
				return err
			}
			fmt.Printf("Pruned %d failed job(s) older than %d hour(s).\n", pruned, hours)
			return nil
		},
	}
	cmd.Flags().String("connection", "", "Queue connection to use")
	cmd.Flags().Int("hours", 24, "Delete failed jobs older than this many hours")
	return cmd
}
