package queue

import (
	"sync"
	"sync/atomic"
	"time"
)

// MemoryQueue is an in-process queue driver backed by memory. Jobs survive
// only for the lifetime of the process; use the database driver for
// durability.
type MemoryQueue struct {
	mu     sync.Mutex
	nextID int64
	jobs   map[string][]*memoryJob // keyed by queue name
	failed []FailedJob
	failID int64
}

type memoryJob struct {
	id          int64
	name        string
	payload     []byte
	attempts    int
	availableAt time.Time
}

// NewMemoryQueue creates a new in-memory queue.
func NewMemoryQueue() *MemoryQueue {
	return &MemoryQueue{jobs: make(map[string][]*memoryJob)}
}

// Push dispatches a job onto the default queue.
func (q *MemoryQueue) Push(job Job) error {
	return q.Later(0, job)
}

// Later dispatches a job to become available after the given delay.
func (q *MemoryQueue) Later(delay time.Duration, job Job) error {
	name, body, err := marshalJob(job)
	if err != nil {
		return err
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	q.nextID++
	q.jobs["default"] = append(q.jobs["default"], &memoryJob{
		id:          q.nextID,
		name:        name,
		payload:     body,
		availableAt: time.Now().Add(delay),
	})
	return nil
}

// Pop reserves the next available job by removing it from the queue.
func (q *MemoryQueue) Pop(queue string) (*ReservedJob, error) {
	if queue == "" {
		queue = "default"
	}
	q.mu.Lock()
	defer q.mu.Unlock()

	now := time.Now()
	list := q.jobs[queue]
	for i, j := range list {
		if j.availableAt.After(now) {
			continue
		}
		q.jobs[queue] = append(list[:i], list[i+1:]...)
		j.attempts++
		return &ReservedJob{
			ID:       j.id,
			Queue:    queue,
			Name:     j.name,
			Payload:  j.payload,
			Attempts: j.attempts,
		}, nil
	}
	return nil, nil
}

// Delete is a no-op: Pop already removed the job.
func (q *MemoryQueue) Delete(_ *ReservedJob) error {
	return nil
}

// Release returns a job to the queue for another attempt after a delay.
func (q *MemoryQueue) Release(job *ReservedJob, delay time.Duration) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.jobs[job.Queue] = append(q.jobs[job.Queue], &memoryJob{
		id:          job.ID,
		name:        job.Name,
		payload:     job.Payload,
		attempts:    job.Attempts,
		availableAt: time.Now().Add(delay),
	})
	return nil
}

// Fail records the job in the in-memory failed list.
func (q *MemoryQueue) Fail(job *ReservedJob, jobErr error) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	id := atomic.AddInt64(&q.failID, 1)
	q.failed = append(q.failed, FailedJob{
		ID:        id,
		Queue:     job.Queue,
		Name:      job.Name,
		Payload:   job.Payload,
		Exception: jobErr.Error(),
		FailedAt:  time.Now(),
	})
	return nil
}

// ListFailed returns all failed jobs.
func (q *MemoryQueue) ListFailed() ([]FailedJob, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	return append([]FailedJob(nil), q.failed...), nil
}

// RetryFailed re-dispatches a failed job.
func (q *MemoryQueue) RetryFailed(id int64) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	for i, f := range q.failed {
		if f.ID != id {
			continue
		}
		q.failed = append(q.failed[:i], q.failed[i+1:]...)
		q.nextID++
		q.jobs[f.Queue] = append(q.jobs[f.Queue], &memoryJob{
			id:          q.nextID,
			name:        f.Name,
			payload:     f.Payload,
			availableAt: time.Now(),
		})
		return nil
	}
	return nil
}

// ForgetFailed removes a failed job without retrying it.
func (q *MemoryQueue) ForgetFailed(id int64) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	for i, f := range q.failed {
		if f.ID == id {
			q.failed = append(q.failed[:i], q.failed[i+1:]...)
			return nil
		}
	}
	return nil
}

// Size returns the number of pending jobs on a queue.
func (q *MemoryQueue) Size(queue string) int {
	if queue == "" {
		queue = "default"
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.jobs[queue])
}
