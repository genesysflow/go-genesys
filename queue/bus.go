package queue

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
)

// QueueAware jobs receive the queue they were popped from before they
// run, letting them dispatch follow-up work. The worker injects it.
type QueueAware interface {
	SetQueue(q Queue)
}

// --- chains ---

// ChainJob runs a sequence of jobs one at a time: each link runs, and
// only on success is the next link dispatched back onto the queue. A
// failing link stops the chain (normal retry/backoff still applies to
// the chain job carrying it).
type ChainJob struct {
	Payloads [][]byte `json:"payloads"`

	queue Queue
}

// JobName gives chains a stable registered name.
func (c *ChainJob) JobName() string { return "genesys.chain" }

// SetQueue receives the popped-from queue (QueueAware).
func (c *ChainJob) SetQueue(q Queue) { c.queue = q }

// Handle runs the first link and dispatches the remainder.
func (c *ChainJob) Handle() error {
	if len(c.Payloads) == 0 {
		return nil
	}
	job, name, err := unmarshalJob(c.Payloads[0])
	if err != nil {
		return err
	}
	if aware, ok := job.(QueueAware); ok && c.queue != nil {
		aware.SetQueue(c.queue)
	}
	if err := job.Handle(); err != nil {
		return fmt.Errorf("chain link %s: %w", name, err)
	}
	if len(c.Payloads) == 1 {
		return nil
	}
	rest := &ChainJob{Payloads: c.Payloads[1:], queue: c.queue}
	if c.queue == nil {
		// No queue to continue on (job ran inline): run the rest inline.
		return rest.Handle()
	}
	return c.queue.Push(rest)
}

// Chain dispatches jobs to run strictly one after another; a failure
// stops the remaining links:
//
//	queue.Chain(q, &ProcessPodcast{}, &OptimizePodcast{}, &ReleasePodcast{})
func Chain(q Queue, jobs ...Job) error {
	if len(jobs) == 0 {
		return nil
	}
	payloads := make([][]byte, len(jobs))
	for i, job := range jobs {
		_, body, err := marshalJob(job)
		if err != nil {
			return err
		}
		payloads[i] = body
	}
	chain := &ChainJob{Payloads: payloads, queue: q}
	return q.Push(chain)
}

// --- batches ---

// Batch tracks a group of jobs dispatched together, with completion
// callbacks - Laravel's Bus::batch. Progress lives in this process, so
// callbacks fire when the batch's jobs are worked in the same process
// (any driver); workers in other processes still run the jobs but
// cannot fire this instance's callbacks.
type Batch struct {
	// ID identifies the batch.
	ID string

	mu        sync.Mutex
	total     int
	completed int
	failed    int
	errs      []error

	then     func(*Batch)
	catch    func(*Batch, error)
	finally_ func(*Batch)
}

var batchRegistry sync.Map // id -> *Batch

// NewBatch prepares a batch of jobs; nothing runs until Dispatch.
func NewBatch() *Batch {
	raw := make([]byte, 16)
	rand.Read(raw)
	return &Batch{ID: hex.EncodeToString(raw)}
}

// Then registers a callback fired when every job succeeded.
func (b *Batch) Then(fn func(*Batch)) *Batch { b.then = fn; return b }

// Catch registers a callback fired on the first job failure.
func (b *Batch) Catch(fn func(*Batch, error)) *Batch { b.catch = fn; return b }

// Finally registers a callback fired when all jobs settled, either way.
func (b *Batch) Finally(fn func(*Batch)) *Batch { b.finally_ = fn; return b }

// Dispatch wraps and pushes the jobs onto the queue. When a batch
// repository is configured (SetBatchRepository) the batch's counters
// are persisted so other processes can track it via FindBatch.
func (b *Batch) Dispatch(q Queue, jobs ...Job) error {
	if len(jobs) == 0 {
		return nil
	}
	b.mu.Lock()
	firstDispatch := b.total == 0
	b.total += len(jobs)
	b.mu.Unlock()
	batchRegistry.Store(b.ID, b)

	if repo := currentBatchRepository(); repo != nil {
		var err error
		if firstDispatch {
			err = repo.Insert(b.ID, len(jobs))
		} else {
			err = repo.Add(b.ID, len(jobs))
		}
		if err != nil {
			return err
		}
	}

	for _, job := range jobs {
		_, body, err := marshalJob(job)
		if err != nil {
			return err
		}
		if err := q.Push(&batchJob{BatchID: b.ID, Payload: body}); err != nil {
			return err
		}
	}
	return nil
}

// Progress returns completed and total counts.
func (b *Batch) Progress() (completed, total int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.completed + b.failed, b.total
}

// FailedCount returns how many jobs failed.
func (b *Batch) FailedCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.failed
}

// Finished reports whether every job has settled.
func (b *Batch) Finished() bool {
	settled, total := b.Progress()
	return total > 0 && settled >= total
}

// record notes one settled job and fires callbacks when done.
func (b *Batch) record(jobErr error) {
	b.mu.Lock()
	if jobErr != nil {
		b.failed++
		b.errs = append(b.errs, jobErr)
	} else {
		b.completed++
	}
	done := b.completed+b.failed >= b.total
	firstErr := error(nil)
	if len(b.errs) > 0 {
		firstErr = b.errs[0]
	}
	then, catch, finally := b.then, b.catch, b.finally_
	failed := b.failed
	b.mu.Unlock()

	if !done {
		return
	}
	batchRegistry.Delete(b.ID)
	if failed > 0 {
		if catch != nil {
			catch(b, firstErr)
		}
	} else if then != nil {
		then(b)
	}
	if finally != nil {
		finally(b)
	}
}

// batchJob wraps one job of a batch.
type batchJob struct {
	BatchID string `json:"batch_id"`
	Payload []byte `json:"payload"`

	queue Queue
}

// JobName gives batch wrappers a stable registered name.
func (j *batchJob) JobName() string { return "genesys.batch" }

// SetQueue receives the popped-from queue (QueueAware).
func (j *batchJob) SetQueue(q Queue) { j.queue = q }

// Handle runs the wrapped job; the outcome reaches the batch through
// settled once the worker decides it is final.
func (j *batchJob) Handle() error {
	job, name, err := unmarshalJob(j.Payload)
	if err != nil {
		return err
	}
	if aware, ok := job.(QueueAware); ok && j.queue != nil {
		aware.SetQueue(j.queue)
	}
	if jobErr := job.Handle(); jobErr != nil {
		return fmt.Errorf("batch job %s: %w", name, jobErr)
	}
	return nil
}

// settled reports the outcome to the batch only when it is terminal -
// a release for retry must not settle the job, or a retried failure
// would finish the batch early and double-count on later attempts.
func (j *batchJob) settled(finalErr error, willRetry bool) {
	if willRetry {
		return
	}
	j.report(finalErr)
}

func (j *batchJob) report(jobErr error) {
	// Persist progress for cross-process visibility; the row is kept
	// after completion so FindBatch still answers.
	if repo := currentBatchRepository(); repo != nil {
		repo.Increment(j.BatchID, jobErr != nil)
	}
	if stored, ok := batchRegistry.Load(j.BatchID); ok {
		stored.(*Batch).record(jobErr)
	}
}

func init() {
	Register[ChainJob]()
	Register[batchJob]()
}
