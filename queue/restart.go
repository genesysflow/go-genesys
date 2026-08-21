package queue

import (
	"errors"
	"fmt"
	"time"

	"github.com/genesysflow/go-genesys/cache"
)

// ErrRestartRequested is returned by Worker.Run when a restart signal
// issued after the worker started is observed. A supervisor (systemd,
// Kubernetes, a process manager) then starts a fresh worker, which is
// how a deploy gets new code onto the queue without killing a job
// mid-flight.
var ErrRestartRequested = errors.New("queue: restart requested")

// restartKey holds the timestamp of the last restart signal.
const restartKey = "genesys:queue:restart"

// SignalRestart asks every worker watching this store to stop once it
// finishes what it is doing, Laravel's `queue:restart`.
func SignalRestart(store cache.Store) error {
	if store == nil {
		return fmt.Errorf("queue: a cache store is required to signal a restart")
	}
	return store.Forever(restartKey, time.Now().UnixNano())
}

// RestartSignalledAt returns when a restart was last requested, and
// whether one ever was.
func RestartSignalledAt(store cache.Store) (time.Time, bool) {
	if store == nil {
		return time.Time{}, false
	}

	value, err := store.Get(restartKey)
	if err != nil || value == nil {
		return time.Time{}, false
	}

	switch stamp := value.(type) {
	case int64:
		return time.Unix(0, stamp), true
	case int:
		return time.Unix(0, int64(stamp)), true
	case float64:
		return time.Unix(0, int64(stamp)), true
	default:
		return time.Time{}, false
	}
}

// WatchRestart makes the worker honour restart signals sent through the
// store. The worker stops between jobs, never mid-job, and only for a
// signal issued after it started - a signal from an earlier deploy must
// not stop the worker that replaced it.
func (w *Worker) WatchRestart(store cache.Store) *Worker {
	w.restartStore = store
	w.startedAt = time.Now()
	return w
}

// shouldRestart reports whether a restart was requested since this
// worker started.
func (w *Worker) shouldRestart() bool {
	if w.restartStore == nil {
		return false
	}

	signalled, ok := RestartSignalledAt(w.restartStore)
	return ok && signalled.After(w.startedAt)
}

// SizeProvider is implemented by drivers that can report how many jobs
// are waiting, backing `queue:monitor`. Not every driver can: a sync
// driver has no backlog to count.
type SizeProvider interface {
	// Size returns the number of jobs waiting on a queue. An empty name
	// means the driver's default queue.
	Size(queue string) (int64, error)
}
