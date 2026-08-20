package queue

import "time"

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

// Nameable lets a job override its registered name.
type Nameable interface {
	// JobName returns the stable name the job is registered under.
	JobName() string
}
