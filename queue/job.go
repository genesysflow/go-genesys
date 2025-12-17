package queue

// Job is the interface that all jobs must implement.
type Job interface {
	// Handle executes the job.
	Handle() error
}
