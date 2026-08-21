package queue

import "time"

// SyncQueue is a synchronous queue driver.
// It executes jobs immediately in the dispatching goroutine.
type SyncQueue struct{}

// NewSyncQueue creates a new synchronous queue.
func NewSyncQueue() *SyncQueue {
	return &SyncQueue{}
}

// Push executes the job immediately. Inline runs settle in one shot,
// so batch wrappers report their outcome right away.
func (q *SyncQueue) Push(job Job) error {
	err := job.Handle()
	notifySettled(job, err, false)
	return err
}

// Later executes the job immediately; the sync driver ignores delays,
// matching Laravel's sync connection.
func (q *SyncQueue) Later(_ time.Duration, job Job) error {
	return q.Push(job)
}

// PushOn runs the job immediately - the sync driver has no named
// queues, matching Laravel's sync connection where onQueue is a no-op.
func (q *SyncQueue) PushOn(queueName string, delay time.Duration, job Job) error {
	return q.Push(job)
}
