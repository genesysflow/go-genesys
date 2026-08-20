package queue

import "time"

// SyncQueue is a synchronous queue driver.
// It executes jobs immediately in the dispatching goroutine.
type SyncQueue struct{}

// NewSyncQueue creates a new synchronous queue.
func NewSyncQueue() *SyncQueue {
	return &SyncQueue{}
}

// Push executes the job immediately.
func (q *SyncQueue) Push(job Job) error {
	return job.Handle()
}

// Later executes the job immediately; the sync driver ignores delays,
// matching Laravel's sync connection.
func (q *SyncQueue) Later(_ time.Duration, job Job) error {
	return job.Handle()
}
