package queue

import (
	"context"
	"time"
)

// Job is the interface that all jobs must implement.
type Job interface {
	// Handle executes the job.
	Handle() error
}

// HasTries lets a job override the worker's default max attempts.
type HasTries interface {
	// Tries returns the maximum number of attempts for this job.
	Tries() int
}

// HasBackoff lets a job override the worker's default retry delay.
type HasBackoff interface {
	// Backoff returns the delay before a failed attempt is retried.
	Backoff() time.Duration
}

// HasTimeout lets a job override the worker's default per-job timeout.
type HasTimeout interface {
	Timeout() time.Duration
}

// ContextJob is an optional, context-aware handler. When the worker
// enforces a timeout it prefers HandleContext, passing a context whose
// deadline is the timeout, so the job can stop its own work cleanly.
type ContextJob interface {
	HandleContext(ctx context.Context) error
}

// Nameable lets a job override its registered name.
type Nameable interface {
	// JobName returns the stable name the job is registered under.
	JobName() string
}
