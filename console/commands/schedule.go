package commands

import (
	"context"
	"fmt"
	"os/signal"
	"syscall"
	"time"

	"github.com/genesysflow/go-genesys/container"
	"github.com/genesysflow/go-genesys/contracts"
	"github.com/genesysflow/go-genesys/schedule"
	"github.com/spf13/cobra"
)

func resolveSchedule(app contracts.Application) (*schedule.Schedule, error) {
	if err := app.Boot(); err != nil {
		return nil, fmt.Errorf("failed to boot application: %w", err)
	}
	scheduler, err := container.Resolve[*schedule.Schedule](app)
	if err != nil {
		return nil, fmt.Errorf("scheduler not available - register the ScheduleServiceProvider: %w", err)
	}
	return scheduler, nil
}

// ScheduleRunCommand creates the schedule:run command (run due tasks once;
// call it from system cron every minute).
func ScheduleRunCommand(app contracts.Application) *cobra.Command {
	return &cobra.Command{
		Use:   "schedule:run",
		Short: "Run scheduled tasks that are due now",
		RunE: func(cmd *cobra.Command, args []string) error {
			scheduler, err := resolveSchedule(app)
			if err != nil {
				return err
			}

			results := scheduler.RunDue(time.Now())
			if len(results) == 0 {
				fmt.Println("No scheduled tasks are due.")
				return nil
			}
			for label, err := range results {
				if err != nil {
					fmt.Printf("FAIL %s: %v\n", label, err)
				} else {
					fmt.Printf("OK   %s\n", label)
				}
			}
			return nil
		},
	}
}

// ScheduleWorkCommand creates the schedule:work command (long-running
// scheduler loop).
func ScheduleWorkCommand(app contracts.Application) *cobra.Command {
	return &cobra.Command{
		Use:   "schedule:work",
		Short: "Run the scheduler in the foreground, evaluating every minute",
		RunE: func(cmd *cobra.Command, args []string) error {
			scheduler, err := resolveSchedule(app)
			if err != nil {
				return err
			}

			ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()

			fmt.Println("Schedule worker started. Press Ctrl+C to stop.")
			err = scheduler.Work(ctx, func(label string, err error) {
				if err != nil {
					fmt.Printf("FAIL %s: %v\n", label, err)
				} else {
					fmt.Printf("OK   %s\n", label)
				}
			})
			if err != nil && err != context.Canceled {
				return err
			}
			fmt.Println("Schedule worker stopped.")
			return nil
		},
	}
}

// ScheduleListCommand creates the schedule:list command.
func ScheduleListCommand(app contracts.Application) *cobra.Command {
	return &cobra.Command{
		Use:   "schedule:list",
		Short: "List all scheduled tasks",
		RunE: func(cmd *cobra.Command, args []string) error {
			scheduler, err := resolveSchedule(app)
			if err != nil {
				return err
			}

			events := scheduler.Events()
			if len(events) == 0 {
				fmt.Println("No scheduled tasks defined.")
				return nil
			}
			for _, event := range events {
				description := event.GetDescription()
				if description == "" {
					description = "(no description)"
				}
				fmt.Printf("%-20s %s\n", event.Expression(), description)
			}
			return nil
		},
	}
}
