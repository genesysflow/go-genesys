package commands

import (
	"context"
	"fmt"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/genesysflow/go-genesys/container"
	"github.com/genesysflow/go-genesys/contracts"
	"github.com/genesysflow/go-genesys/queue"
	"github.com/spf13/cobra"
)

func resolveQueueConnection(app contracts.Application, name string) (queue.Queue, error) {
	if err := app.Boot(); err != nil {
		return nil, fmt.Errorf("failed to boot application: %w", err)
	}
	manager, err := container.Resolve[*queue.Manager](app)
	if err != nil {
		return nil, fmt.Errorf("queue manager not available: %w", err)
	}
	return manager.Connection(name)
}

// QueueWorkCommand creates the queue:work command.
func QueueWorkCommand(app contracts.Application) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "queue:work",
		Short: "Process jobs from the queue",
		RunE: func(cmd *cobra.Command, args []string) error {
			connName, _ := cmd.Flags().GetString("connection")
			queueName, _ := cmd.Flags().GetString("queue")
			sleep, _ := cmd.Flags().GetDuration("sleep")
			tries, _ := cmd.Flags().GetInt("tries")
			once, _ := cmd.Flags().GetBool("once")
			stopWhenEmpty, _ := cmd.Flags().GetBool("stop-when-empty")

			conn, err := resolveQueueConnection(app, connName)
			if err != nil {
				return err
			}
			driver, ok := conn.(queue.Driver)
			if !ok {
				return fmt.Errorf("queue connection does not support workers (the sync driver runs jobs inline)")
			}

			worker := queue.NewWorker(driver)
			worker.Queue = queueName
			worker.Sleep = sleep
			worker.Tries = tries
			worker.OnError = func(err error) {
				fmt.Printf("[queue] %v\n", err)
			}

			if once {
				_, err := worker.RunOnce()
				return err
			}
			if stopWhenEmpty {
				return worker.Drain()
			}

			ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()

			fmt.Printf("Processing jobs from the [%s] queue. Press Ctrl+C to stop.\n", displayQueueName(queueName))
			if err := worker.Run(ctx); err != nil && err != context.Canceled {
				return err
			}
			fmt.Println("Worker stopped gracefully.")
			return nil
		},
	}

	cmd.Flags().String("connection", "", "Queue connection to use (default from config)")
	cmd.Flags().String("queue", "", "Queue name to process")
	cmd.Flags().Duration("sleep", time.Second, "Sleep duration when the queue is empty")
	cmd.Flags().Int("tries", 3, "Max attempts per job before it is marked failed")
	cmd.Flags().Bool("once", false, "Process a single job and exit")
	cmd.Flags().Bool("stop-when-empty", false, "Process jobs until the queue is empty, then exit")

	return cmd
}

func displayQueueName(name string) string {
	if name == "" {
		return "default"
	}
	return name
}

// QueueFailedCommand creates the queue:failed command.
func QueueFailedCommand(app contracts.Application) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "queue:failed",
		Short: "List failed queue jobs",
		RunE: func(cmd *cobra.Command, args []string) error {
			connName, _ := cmd.Flags().GetString("connection")
			conn, err := resolveQueueConnection(app, connName)
			if err != nil {
				return err
			}
			provider, ok := conn.(queue.FailedJobProvider)
			if !ok {
				return fmt.Errorf("queue connection does not track failed jobs")
			}

			failed, err := provider.ListFailed()
			if err != nil {
				return err
			}
			if len(failed) == 0 {
				fmt.Println("No failed jobs.")
				return nil
			}
			for _, f := range failed {
				fmt.Printf("[%d] %s (queue: %s, failed: %s)\n    %s\n",
					f.ID, f.Name, f.Queue, f.FailedAt.Format(time.RFC3339), f.Exception)
			}
			return nil
		},
	}
	cmd.Flags().String("connection", "", "Queue connection to use")
	return cmd
}

// QueueRetryCommand creates the queue:retry command.
func QueueRetryCommand(app contracts.Application) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "queue:retry <id|all>",
		Short: "Retry failed queue jobs",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			connName, _ := cmd.Flags().GetString("connection")
			conn, err := resolveQueueConnection(app, connName)
			if err != nil {
				return err
			}
			provider, ok := conn.(queue.FailedJobProvider)
			if !ok {
				return fmt.Errorf("queue connection does not track failed jobs")
			}

			if args[0] == "all" {
				failed, err := provider.ListFailed()
				if err != nil {
					return err
				}
				for _, f := range failed {
					if err := provider.RetryFailed(f.ID); err != nil {
						return err
					}
					fmt.Printf("Retried job [%d] %s\n", f.ID, f.Name)
				}
				return nil
			}

			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid job id %q", args[0])
			}
			if err := provider.RetryFailed(id); err != nil {
				return err
			}
			fmt.Printf("Retried job [%d]\n", id)
			return nil
		},
	}
	cmd.Flags().String("connection", "", "Queue connection to use")
	return cmd
}
