package queue

// Queue is the interface for queue drivers.
type Queue interface {
	// Push pushes a job onto the queue.
	Push(job Job) error
}
