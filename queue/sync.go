package queue

// SyncQueue is a synchronous queue driver.
// It executes jobs immediately.
type SyncQueue struct{}

// NewSyncQueue creates a new synchronous queue.
func NewSyncQueue() *SyncQueue {
	return &SyncQueue{}
}

// Push pushes a job onto the queue.
func (q *SyncQueue) Push(job Job) error {
	return job.Handle()
}
