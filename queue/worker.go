package queue

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/genesysflow/go-genesys/cache"
)

// Worker processes jobs popped from a queue driver.
type Worker struct {
	driver Driver

	// Queue is the queue name to process (driver default when empty).
	Queue string

	// Queues, when set, is a priority-ordered list of queues: each poll
	// drains the first queue with a ready job before falling through to
	// the next - Laravel's queue:work --queue=high,default. Overrides
	// Queue.
	Queues []string

	// Sleep is how long to wait when the queue is empty (default 1s).
	Sleep time.Duration

	// Tries is the default max attempts per job (default 3); jobs can
	// override via the HasTries interface.
	Tries int

	// Backoff is the default delay before retrying a failed job
	// (default 5s); jobs can override via the HasBackoff interface.
	Backoff time.Duration

	// Timeout bounds each job's execution (0 = unlimited); jobs can
	// override via the HasTimeout interface. A timed-out job counts as
	// a failed attempt and follows the normal retry/fail path. Jobs
	// implementing ContextJob receive the deadline through their
	// context; plain Handle() jobs are abandoned to finish in the
	// background while the worker moves on.
	Timeout time.Duration

	// OnError, when set, is called with processing errors (job failures
	// and infrastructure errors) so callers can log them.
	OnError func(err error)

	// middleware wraps every job's execution (see Use).
	middleware []JobMiddleware

	// restart signalling (see WatchRestart).
	restartStore cache.Store
	startedAt    time.Time
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
	if w.startedAt.IsZero() {
		w.startedAt = time.Now()
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Checked between jobs, so a restart never interrupts work in
		// flight.
		if w.shouldRestart() {
			return ErrRestartRequested
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
	reserved, err := w.pop()
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
		if errors.Is(jobErr, ErrOverlapping) {
			// Blocked by a running sibling, not failed: put it back
			// without consuming an attempt.
			reserved.Attempts--
			notifySettled(job, jobErr, true)
			return true, w.driver.Release(reserved, w.backoffFor(job))
		}
		w.report(fmt.Errorf("queue: job %s failed (attempt %d): %w", reserved.Name, reserved.Attempts, jobErr))
		if reserved.Attempts >= w.triesFor(job) {
			notifySettled(job, jobErr, false)
			if err := w.driver.Fail(reserved, jobErr); err != nil {
				return true, err
			}
			return true, nil
		}
		notifySettled(job, jobErr, true)
		if err := w.driver.Release(reserved, w.backoffFor(job)); err != nil {
			return true, err
		}
		return true, nil
	}

	notifySettled(job, nil, false)
	return true, w.driver.Delete(reserved)
}

// settlementAware jobs (batch wrappers) learn the final outcome of a
// run once the worker has decided it; willRetry marks a release for
// another attempt rather than a terminal result.
type settlementAware interface {
	settled(finalErr error, willRetry bool)
}

func notifySettled(job Job, err error, willRetry bool) {
	if aware, ok := job.(settlementAware); ok {
		aware.settled(err, willRetry)
	}
}

// pop reserves the next job, honouring the priority list when set.
func (w *Worker) pop() (*ReservedJob, error) {
	if len(w.Queues) == 0 {
		return w.driver.Pop(w.Queue)
	}
	for _, queueName := range w.Queues {
		reserved, err := w.driver.Pop(queueName)
		if err != nil || reserved != nil {
			return reserved, err
		}
	}
	return nil, nil
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

func (w *Worker) timeoutFor(job Job) time.Duration {
	if withTimeout, ok := job.(HasTimeout); ok {
		if timeout := withTimeout.Timeout(); timeout > 0 {
			return timeout
		}
	}
	return w.Timeout
}

func (w *Worker) report(err error) {
	if w.OnError != nil {
		w.OnError(err)
	}
}

// ErrTimeout marks a job attempt that exceeded its timeout; it counts
// as a normal failure for retry accounting.
var ErrTimeout = fmt.Errorf("queue: job timed out")

// runWithTimeout runs a job with an optional deadline. Context-aware
// jobs get the deadline through their context; plain jobs are run in a
// goroutine and abandoned when the deadline passes (Go cannot kill a
// goroutine), so long-running plain jobs should implement ContextJob.
func runWithTimeout(job Job, timeout time.Duration) error {
	if timeout <= 0 {
		if aware, ok := job.(ContextJob); ok {
			return safeHandleContext(aware, context.Background())
		}
		return safeHandle(job)
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		if aware, ok := job.(ContextJob); ok {
			done <- safeHandleContext(aware, ctx)
			return
		}
		done <- safeHandle(job)
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return fmt.Errorf("%w after %s", ErrTimeout, timeout)
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

// safeHandleContext is safeHandle for context-aware jobs.
func safeHandleContext(job ContextJob, ctx context.Context) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	return job.HandleContext(ctx)
}
