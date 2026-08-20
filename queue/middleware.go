package queue

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

// JobMiddleware wraps job execution on the worker, Laravel's job
// middleware: rate limiting, overlap prevention, logging, etc.
type JobMiddleware func(job Job, next func() error) error

// HasMiddleware lets a job declare its own middleware, which runs after
// the worker's middleware.
type HasMiddleware interface {
	Middleware() []JobMiddleware
}

// Use appends worker-level middleware applied to every job.
func (w *Worker) Use(middleware ...JobMiddleware) *Worker {
	w.middleware = append(w.middleware, middleware...)
	return w
}

// runJob executes a job through the worker's and the job's middleware.
func (w *Worker) runJob(job Job) error {
	handler := func() error { return safeHandle(job) }

	chain := w.middleware
	if withOwn, ok := job.(HasMiddleware); ok {
		chain = append(append([]JobMiddleware(nil), chain...), withOwn.Middleware()...)
	}
	for i := len(chain) - 1; i >= 0; i-- {
		middleware, next := chain[i], handler
		handler = func() error { return middleware(job, next) }
	}
	return handler()
}

// Locker is the minimal lock surface the overlap/unique helpers need;
// cache stores satisfy it.
type Locker interface {
	Add(key string, value any, ttl time.Duration) (bool, error)
	Forget(key string) error
}

// ErrOverlapping is returned when WithoutOverlapping finds another
// instance of the job still running; the worker's normal retry/backoff
// then re-attempts it later.
var ErrOverlapping = fmt.Errorf("queue: job overlaps a running instance")

// WithoutOverlapping prevents two jobs sharing a key from running at
// the same time, using an atomic lock in the given store. ttl bounds
// how long a crashed job can hold the slot:
//
//	worker.Use(queue.WithoutOverlapping(store, time.Minute))
//
// Jobs choose their key via the OverlapKey interface; jobs without one
// run unrestricted.
func WithoutOverlapping(locks Locker, ttl time.Duration) JobMiddleware {
	return func(job Job, next func() error) error {
		keyed, ok := job.(OverlapKey)
		if !ok {
			return next()
		}
		key := "queue:overlap:" + keyed.OverlapKey()
		acquired, err := locks.Add(key, 1, ttl)
		if err != nil {
			return next() // a broken lock store must not stop the queue
		}
		if !acquired {
			return ErrOverlapping
		}
		defer locks.Forget(key)
		return next()
	}
}

// OverlapKey gives a job a mutual-exclusion key for WithoutOverlapping.
type OverlapKey interface {
	OverlapKey() string
}

// UniqueID gives a job an explicit uniqueness key for DispatchUnique;
// without it the key is derived from the registered name and payload.
type UniqueID interface {
	UniqueID() string
}

// DispatchUnique pushes a job unless an identical one was dispatched in
// the last ttl - Laravel's ShouldBeUnique. Reports whether the job was
// actually pushed:
//
//	pushed, err := queue.DispatchUnique(q, store, &RebuildIndex{}, time.Minute)
func DispatchUnique(q Queue, locks Locker, job Job, ttl time.Duration) (bool, error) {
	name, body, err := marshalJob(job)
	if err != nil {
		return false, err
	}

	key := ""
	if unique, ok := job.(UniqueID); ok {
		key = unique.UniqueID()
	} else {
		sum := sha256.Sum256(body)
		key = name + ":" + hex.EncodeToString(sum[:8])
	}

	acquired, err := locks.Add("queue:unique:"+key, 1, ttl)
	if err != nil {
		return false, err
	}
	if !acquired {
		return false, nil
	}
	if err := q.Push(job); err != nil {
		locks.Forget("queue:unique:" + key)
		return false, err
	}
	return true, nil
}
