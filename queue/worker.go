package queue

import (
	"context"
	"fmt"
	"time"
)

// Worker processes jobs popped from a queue driver.
type Worker struct {
	driver Driver

	// Queue is the queue name to process (driver default when empty).
	Queue string

	// Sleep is how long to wait when the queue is empty (default 1s).
	Sleep time.Duration

	// Tries is the default max attempts per job (default 3); jobs can
	// override via the HasTries interface.
	Tries int

	// Backoff is the default delay before retrying a failed job
	// (default 5s); jobs can override via the HasBackoff interface.
	Backoff time.Duration

	// OnError, when set, is called with processing errors (job failures
	// and infrastructure errors) so callers can log them.
	OnError func(err error)

	// middleware wraps every job's execution (see Use).
	middleware []JobMiddleware
}

// NewWorker creates a worker for the given driver.
func NewWorker(driver Driver) *Worker {
	return &Worker{
		driver:  driver,
		Sleep:   time.Second,
		Tries:   3,
		Backoff: 5 * time.Second,
	}
}

// Run processes jobs until the context is cancelled.
func (w *Worker) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		processed, err := w.RunOnce()
		if err != nil {
			w.report(err)
		}
		if !processed {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(w.Sleep):
			}
		}
	}
}

// RunOnce pops and processes a single job. It returns whether a job was
// processed; job failures are handled (release or fail) and reported via
// OnError, not returned.
func (w *Worker) RunOnce() (bool, error) {
	reserved, err := w.driver.Pop(w.Queue)
	if err != nil {
		return false, err
	}
	if reserved == nil {
		return false, nil
	}

	job, _, err := unmarshalJob(reserved.Payload)
	if err != nil {
		// Unresolvable payloads can never succeed: fail them immediately.
		if failErr := w.driver.Fail(reserved, err); failErr != nil {
			return true, failErr
		}
		w.report(err)
		return true, nil
	}

	// Give queue-aware jobs (chains, batches) a handle back to the
	// driver so they can dispatch their continuation.
	if aware, ok := job.(QueueAware); ok {
		aware.SetQueue(w.driver)
	}

	if jobErr := w.runJob(job); jobErr != nil {
		w.report(fmt.Errorf("queue: job %s failed (attempt %d): %w", reserved.Name, reserved.Attempts, jobErr))
		if reserved.Attempts >= w.triesFor(job) {
			if err := w.driver.Fail(reserved, jobErr); err != nil {
				return true, err
			}
			return true, nil
		}
		if err := w.driver.Release(reserved, w.backoffFor(job)); err != nil {
			return true, err
		}
		return true, nil
	}

	return true, w.driver.Delete(reserved)
}

// Drain processes jobs until the queue is empty; useful in tests and for
// one-shot processing (queue:work --once drains a single job instead).
func (w *Worker) Drain() error {
	for {
		processed, err := w.RunOnce()
		if err != nil {
			return err
		}
		if !processed {
			return nil
		}
	}
}

func (w *Worker) triesFor(job Job) int {
	if withTries, ok := job.(HasTries); ok {
		if tries := withTries.Tries(); tries > 0 {
			return tries
		}
	}
	if w.Tries > 0 {
		return w.Tries
	}
	return 1
}

func (w *Worker) backoffFor(job Job) time.Duration {
	if withBackoff, ok := job.(HasBackoff); ok {
		return withBackoff.Backoff()
	}
	return w.Backoff
}

func (w *Worker) report(err error) {
	if w.OnError != nil {
		w.OnError(err)
	}
}

// safeHandle runs a job, converting panics into errors so a panicking job
// cannot kill the worker.
func safeHandle(job Job) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	return job.Handle()
}
