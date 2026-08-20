// Package queue provides Laravel-style background job processing with
// sync, memory, and database drivers.
package queue

import "time"

// Queue is the interface all queue connections implement.
type Queue interface {
	// Push dispatches a job onto the queue.
	Push(job Job) error

	// Later dispatches a job to run after the given delay.
	Later(delay time.Duration, job Job) error
}

// ReservedJob is a job popped from a driver, ready to be processed.
type ReservedJob struct {
	// ID identifies the job within the driver.
	ID int64

	// Queue is the queue name the job was popped from.
	Queue string

	// Name is the registered job name.
	Name string

	// Payload is the JSON-encoded job data.
	Payload []byte

	// Attempts is the number of times the job has been attempted,
	// including the current attempt.
	Attempts int
}

// Driver is a queue connection that supports workers.
type Driver interface {
	Queue

	// Pop reserves the next available job on the given queue.
	// Returns (nil, nil) when the queue is empty.
	Pop(queue string) (*ReservedJob, error)

	// Delete removes a completed job.
	Delete(job *ReservedJob) error

	// Release returns a failed job to the queue after a delay.
	Release(job *ReservedJob, delay time.Duration) error

	// Fail records a permanently failed job.
	Fail(job *ReservedJob, jobErr error) error
}

// FailedJob is a job that exhausted its attempts.
type FailedJob struct {
	ID        int64     `json:"id"`
	Queue     string    `json:"queue"`
	Name      string    `json:"name"`
	Payload   []byte    `json:"payload"`
	Exception string    `json:"exception"`
	FailedAt  time.Time `json:"failed_at"`
}

// FailedJobProvider is implemented by drivers that record failed jobs.
type FailedJobProvider interface {
	// ListFailed returns all failed jobs.
	ListFailed() ([]FailedJob, error)

	// RetryFailed re-dispatches a failed job by id and removes it from
	// the failed list.
	RetryFailed(id int64) error

	// ForgetFailed removes a failed job without retrying it.
	ForgetFailed(id int64) error
}
