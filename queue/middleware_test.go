package queue_test

import (
	"database/sql"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/genesysflow/go-genesys/cache"
	"github.com/genesysflow/go-genesys/query"
	"github.com/genesysflow/go-genesys/queue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

// --- job middleware ---

type tracedJob struct{ Label string }

var traceMu sync.Mutex
var trace []string

func addTrace(s string) {
	traceMu.Lock()
	defer traceMu.Unlock()
	trace = append(trace, s)
}

func (j *tracedJob) Handle() error {
	addTrace("handle:" + j.Label)
	return nil
}

func (j *tracedJob) Middleware() []queue.JobMiddleware {
	return []queue.JobMiddleware{
		func(job queue.Job, next func() error) error {
			addTrace("job-mw:before")
			defer addTrace("job-mw:after")
			return next()
		},
	}
}

func TestWorkerAndJobMiddlewareOrder(t *testing.T) {
	queue.Register[tracedJob]()
	traceMu.Lock()
	trace = nil
	traceMu.Unlock()

	q := queue.NewMemoryQueue()
	require.NoError(t, q.Push(&tracedJob{Label: "x"}))

	worker := queue.NewWorker(q).Use(func(job queue.Job, next func() error) error {
		addTrace("worker-mw:before")
		defer addTrace("worker-mw:after")
		return next()
	})
	require.NoError(t, worker.Drain())

	assert.Equal(t, []string{
		"worker-mw:before", "job-mw:before", "handle:x", "job-mw:after", "worker-mw:after",
	}, trace, "worker middleware wraps job middleware wraps the handler")
}

// --- WithoutOverlapping ---

type exclusiveJob struct{ Key string }

var running, maxRunning atomic.Int64

func (j *exclusiveJob) Handle() error {
	now := running.Add(1)
	for {
		max := maxRunning.Load()
		if now <= max || maxRunning.CompareAndSwap(max, now) {
			break
		}
	}
	time.Sleep(10 * time.Millisecond)
	running.Add(-1)
	return nil
}

func (j *exclusiveJob) OverlapKey() string { return j.Key }

func TestWithoutOverlapping(t *testing.T) {
	queue.Register[exclusiveJob]()
	running.Store(0)
	maxRunning.Store(0)

	store := cache.NewMemoryStore()
	q := queue.NewMemoryQueue()
	for i := 0; i < 4; i++ {
		require.NoError(t, q.Push(&exclusiveJob{Key: "same"}))
	}

	// Two workers drain concurrently; overlapping instances are refused
	// and retried, so the critical section never doubles up.
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			worker := queue.NewWorker(q).Use(queue.WithoutOverlapping(store, time.Minute))
			worker.Tries = 100
			worker.Backoff = time.Millisecond
			deadline := time.Now().Add(5 * time.Second)
			for q.Size("") > 0 && time.Now().Before(deadline) {
				worker.RunOnce()
				time.Sleep(time.Millisecond)
			}
		}()
	}
	wg.Wait()

	assert.EqualValues(t, 1, maxRunning.Load(), "no two jobs with the same key ran together")
}

// --- unique dispatch ---

type reindexJob struct{ Index string }

func (j *reindexJob) Handle() error    { return nil }
func (j *reindexJob) UniqueID() string { return "reindex:" + j.Index }

func TestDispatchUnique(t *testing.T) {
	queue.Register[reindexJob]()
	store := cache.NewMemoryStore()
	q := queue.NewMemoryQueue()

	pushed, err := queue.DispatchUnique(q, store, &reindexJob{Index: "users"}, time.Minute)
	require.NoError(t, err)
	assert.True(t, pushed)

	// The same unique id within the window is dropped.
	pushed, err = queue.DispatchUnique(q, store, &reindexJob{Index: "users"}, time.Minute)
	require.NoError(t, err)
	assert.False(t, pushed)

	// A different id goes through.
	pushed, err = queue.DispatchUnique(q, store, &reindexJob{Index: "posts"}, time.Minute)
	require.NoError(t, err)
	assert.True(t, pushed)

	assert.Equal(t, 2, q.Size(""))
}

// --- database-backed batches ---

type batchSqliteExec struct{ db *sql.DB }

func (e *batchSqliteExec) Query(q string, b ...any) (*sql.Rows, error) { return e.db.Query(q, b...) }
func (e *batchSqliteExec) QueryRow(q string, b ...any) *sql.Row        { return e.db.QueryRow(q, b...) }
func (e *batchSqliteExec) Exec(q string, b ...any) (sql.Result, error) { return e.db.Exec(q, b...) }

func newBatchRepo(t *testing.T) queue.BatchRepository {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	_, err = db.Exec(`CREATE TABLE job_batches (
		id TEXT PRIMARY KEY, total_jobs INTEGER NOT NULL,
		completed_jobs INTEGER NOT NULL DEFAULT 0,
		failed_jobs INTEGER NOT NULL DEFAULT 0,
		created_at TIMESTAMP NOT NULL)`)
	require.NoError(t, err)
	return queue.NewDatabaseBatchRepository("sqlite", &batchSqliteExec{db: db}, "")
}

func TestDatabaseBackedBatch(t *testing.T) {
	queue.Register[stepJob]()
	resetLog()

	repo := newBatchRepo(t)
	queue.SetBatchRepository(repo)
	t.Cleanup(func() { queue.SetBatchRepository(nil) })

	q := queue.NewMemoryQueue()
	batch := queue.NewBatch()
	require.NoError(t, batch.Dispatch(q,
		&stepJob{Step: "p1"},
		&stepJob{Step: "p2"},
		&stepJob{Step: "bad", Fail: true},
	))

	// Progress is visible from the repository before any work happens.
	progress, err := queue.FindBatch(batch.ID)
	require.NoError(t, err)
	assert.Equal(t, 3, progress.Total)
	assert.False(t, progress.Finished())

	worker := queue.NewWorker(q)
	worker.Tries = 1
	require.NoError(t, worker.Drain())

	// Counters persisted - what another process would see.
	progress, err = queue.FindBatch(batch.ID)
	require.NoError(t, err)
	assert.Equal(t, 3, progress.Total)
	assert.Equal(t, 2, progress.Completed)
	assert.Equal(t, 1, progress.Failed)
	assert.True(t, progress.Finished())

	// Unknown ids error.
	_, err = queue.FindBatch("missing")
	assert.Error(t, err)
}

// Keep the query import for the repository's table type assertions.
var _ = query.New
