package queue_test

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/genesysflow/go-genesys/queue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- crash durability: a popped-but-never-settled job must come back ---

func TestRedisReservationReclaim(t *testing.T) {
	q, _ := newRedisQueue(t)
	q.WithRetryAfter(50 * time.Millisecond)

	require.NoError(t, q.Push(&redisTestJob{Word: "crashy"}))

	first, err := q.Pop("")
	require.NoError(t, err)
	require.NotNil(t, first)
	assert.Equal(t, 1, first.Attempts)

	// While reserved the job is invisible to other workers.
	hidden, err := q.Pop("")
	require.NoError(t, err)
	assert.Nil(t, hidden)
	assert.Equal(t, 1, queueSize(t, q, ""), "reserved jobs still count toward size")

	// The worker "crashes" (never settles). After retry_after the job
	// is reclaimed with its attempt counted.
	time.Sleep(60 * time.Millisecond)
	reclaimed, err := q.Pop("")
	require.NoError(t, err)
	require.NotNil(t, reclaimed, "expired reservation must be reclaimed")
	assert.Equal(t, 2, reclaimed.Attempts)

	require.NoError(t, q.Delete(reclaimed))
	assert.Equal(t, 0, queueSize(t, q, ""))
}

func TestRedisSettledJobIsGone(t *testing.T) {
	q, _ := newRedisQueue(t)
	q.WithRetryAfter(30 * time.Millisecond)

	require.NoError(t, q.Push(&redisTestJob{Word: "done"}))
	job, err := q.Pop("")
	require.NoError(t, err)
	require.NotNil(t, job)
	require.NoError(t, q.Delete(job))

	// A deleted job must not resurrect after retry_after.
	time.Sleep(40 * time.Millisecond)
	back, err := q.Pop("")
	require.NoError(t, err)
	assert.Nil(t, back)
}

func TestDatabaseReservationReclaim(t *testing.T) {
	resetJobState()
	exec := newQueueDB(t)
	q := queue.NewDatabaseQueue("sqlite", exec,
		queue.DatabaseQueueConfig{RetryAfter: 90 * time.Second})

	require.NoError(t, q.Push(&CountJob{Label: "stuck"}))

	first, err := q.Pop("")
	require.NoError(t, err)
	require.NotNil(t, first)
	assert.Equal(t, 1, first.Attempts)

	// Still reserved: nothing to pop.
	hidden, err := q.Pop("")
	require.NoError(t, err)
	assert.Nil(t, hidden)

	// Simulate the reservation ageing past retry_after.
	_, err = exec.Exec("UPDATE jobs SET reserved_at = reserved_at - 3600")
	require.NoError(t, err)

	reclaimed, err := q.Pop("")
	require.NoError(t, err)
	require.NotNil(t, reclaimed, "stale reservation must be claimable again")
	assert.Equal(t, first.ID, reclaimed.ID)
	assert.Equal(t, 2, reclaimed.Attempts)
	require.NoError(t, q.Delete(reclaimed))
}

// --- batches: retried failures must settle exactly once ---

type settleJob struct {
	Label string `json:"label"`
}

var (
	settleMu   sync.Mutex
	settleRuns map[string]int
)

func (j *settleJob) Handle() error {
	settleMu.Lock()
	defer settleMu.Unlock()
	settleRuns[j.Label]++
	if j.Label == "fail" {
		return errors.New("settle job exploded")
	}
	return nil
}

func TestBatchRetriedFailureSettlesOnce(t *testing.T) {
	queue.Register[settleJob]()
	settleMu.Lock()
	settleRuns = map[string]int{}
	settleMu.Unlock()

	q := queue.NewMemoryQueue()
	var catches, finallies atomic.Int64
	batch := queue.NewBatch().
		Catch(func(_ *queue.Batch, _ error) { catches.Add(1) }).
		Finally(func(_ *queue.Batch) { finallies.Add(1) })

	require.NoError(t, batch.Dispatch(q, &settleJob{Label: "fail"}, &settleJob{Label: "ok"}))

	worker := queue.NewWorker(q)
	worker.Tries = 3
	worker.Backoff = 0
	require.NoError(t, worker.Drain())

	// The failing job really ran three times, but settled only once,
	// after its final attempt - so callbacks fired exactly once and
	// counters never exceed the batch total.
	settleMu.Lock()
	assert.Equal(t, 3, settleRuns["fail"])
	assert.Equal(t, 1, settleRuns["ok"])
	settleMu.Unlock()

	assert.True(t, batch.Finished())
	settled, total := batch.Progress()
	assert.Equal(t, 2, total)
	assert.Equal(t, 2, settled, "settlements must equal the batch total")
	assert.Equal(t, 1, batch.FailedCount())
	assert.EqualValues(t, 1, catches.Load(), "Catch fires once, on the terminal failure")
	assert.EqualValues(t, 1, finallies.Load())
}

// --- overlap: a blocked job is postponed, not failed ---

type mapLocker struct {
	mu    sync.Mutex
	locks map[string]bool
}

func newMapLocker() *mapLocker { return &mapLocker{locks: map[string]bool{}} }

func (l *mapLocker) Add(key string, _ any, _ time.Duration) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.locks[key] {
		return false, nil
	}
	l.locks[key] = true
	return true, nil
}

func (l *mapLocker) Forget(key string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.locks, key)
	return nil
}

type postponedJob struct{ Label string }

var postponedRuns atomic.Int64

func (j *postponedJob) Handle() error {
	postponedRuns.Add(1)
	return nil
}
func (j *postponedJob) OverlapKey() string { return "postponed" }

func TestOverlapBlockedJobIsPostponedNotFailed(t *testing.T) {
	queue.Register[postponedJob]()
	postponedRuns.Store(0)

	locks := newMapLocker()
	q := queue.NewMemoryQueue()
	worker := queue.NewWorker(q)
	worker.Tries = 1 // any counted failure would go straight to the failed list
	worker.Backoff = 0
	worker.Use(queue.WithoutOverlapping(locks, time.Minute))

	// A sibling holds the lock: the job must be postponed, not failed,
	// even though Tries is exhausted by a single counted attempt.
	held, err := locks.Add("queue:overlap:postponed", 1, time.Minute)
	require.NoError(t, err)
	require.True(t, held)

	require.NoError(t, q.Push(&postponedJob{Label: "blocked"}))
	processed, err := worker.RunOnce()
	require.NoError(t, err)
	assert.True(t, processed)

	failed, err := q.ListFailed()
	require.NoError(t, err)
	assert.Empty(t, failed, "an overlap-blocked job must not be recorded as failed")
	assert.Equal(t, 1, queueSize(t, q, ""), "the job is back on the queue")
	assert.EqualValues(t, 0, postponedRuns.Load())

	// Sibling finishes; the postponed job runs with a full attempt budget.
	require.NoError(t, locks.Forget("queue:overlap:postponed"))
	require.NoError(t, worker.Drain())
	assert.EqualValues(t, 1, postponedRuns.Load())
}

// --- registry: RegisterJob's explicit name must round-trip ---

type renamedJob struct{ Word string }

var renamedRuns atomic.Int64

func (j *renamedJob) Handle() error {
	renamedRuns.Add(1)
	return nil
}

func TestRegisterJobCustomNameRoundTrip(t *testing.T) {
	renamedRuns.Store(0)
	queue.RegisterJob("emails.send", func() queue.Job { return &renamedJob{} })

	q := queue.NewMemoryQueue()
	require.NoError(t, q.Push(&renamedJob{Word: "hi"}))

	require.NoError(t, queue.NewWorker(q).Drain())
	assert.EqualValues(t, 1, renamedRuns.Load(), "job serialized under its registered name must be resolvable")

	failed, err := q.ListFailed()
	require.NoError(t, err)
	assert.Empty(t, failed)
}

// --- unique jobs: completion frees the slot before the ttl ---

type uniqueSettleJob struct{}

var uniqueSettleRuns atomic.Int64

func (j *uniqueSettleJob) Handle() error {
	uniqueSettleRuns.Add(1)
	return nil
}
func (j *uniqueSettleJob) UniqueID() string { return "unique-settle" }

func TestReleaseUniqueLockFreesSlotOnCompletion(t *testing.T) {
	queue.Register[uniqueSettleJob]()
	uniqueSettleRuns.Store(0)

	locks := newMapLocker()
	q := queue.NewMemoryQueue()
	worker := queue.NewWorker(q)
	worker.Use(queue.ReleaseUniqueLock(locks))

	pushed, err := queue.DispatchUnique(q, locks, &uniqueSettleJob{}, time.Hour)
	require.NoError(t, err)
	require.True(t, pushed)

	// Identical dispatch while queued is refused.
	dup, err := queue.DispatchUnique(q, locks, &uniqueSettleJob{}, time.Hour)
	require.NoError(t, err)
	assert.False(t, dup)

	require.NoError(t, worker.Drain())
	assert.EqualValues(t, 1, uniqueSettleRuns.Load())

	// Completed: the hour-long ttl no longer blocks the next dispatch.
	again, err := queue.DispatchUnique(q, locks, &uniqueSettleJob{}, time.Hour)
	require.NoError(t, err)
	assert.True(t, again, "completion must free the unique slot immediately")
}
