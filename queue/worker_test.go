package queue_test

import (
	"database/sql"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/genesysflow/go-genesys/queue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

var (
	handled     atomic.Int64
	failUntil   atomic.Int64
	alwaysPanic atomic.Bool
)

// CountJob is a test job that counts executions and can be made to fail.
type CountJob struct {
	Label string `json:"label"`
}

func (j *CountJob) Handle() error {
	if alwaysPanic.Load() {
		panic("boom")
	}
	n := handled.Add(1)
	if n <= failUntil.Load() {
		return errors.New("transient failure")
	}
	return nil
}

func (j *CountJob) Backoff() time.Duration { return 0 }

func resetJobState() {
	handled.Store(0)
	failUntil.Store(0)
	alwaysPanic.Store(false)
}

func init() {
	queue.Register[CountJob]()
}

func TestMemoryQueueRoundTrip(t *testing.T) {
	resetJobState()
	q := queue.NewMemoryQueue()

	require.NoError(t, q.Push(&CountJob{Label: "one"}))
	require.NoError(t, q.Push(&CountJob{Label: "two"}))
	assert.Equal(t, 2, q.Size(""))

	worker := queue.NewWorker(q)
	require.NoError(t, worker.Drain())
	assert.EqualValues(t, 2, handled.Load())
	assert.Equal(t, 0, q.Size(""))
}

func TestWorkerRetriesThenFails(t *testing.T) {
	resetJobState()
	failUntil.Store(1000) // always fail
	q := queue.NewMemoryQueue()
	require.NoError(t, q.Push(&CountJob{Label: "doomed"}))

	worker := queue.NewWorker(q)
	worker.Tries = 3
	require.NoError(t, worker.Drain())

	assert.EqualValues(t, 3, handled.Load(), "job should be attempted Tries times")
	failed, err := q.ListFailed()
	require.NoError(t, err)
	require.Len(t, failed, 1)
	assert.Contains(t, failed[0].Exception, "transient failure")

	// Retry the failed job; this time let it succeed.
	failUntil.Store(0)
	handled.Store(0)
	require.NoError(t, q.RetryFailed(failed[0].ID))
	require.NoError(t, worker.Drain())
	assert.EqualValues(t, 1, handled.Load())
	failed, err = q.ListFailed()
	require.NoError(t, err)
	assert.Empty(t, failed)
}

func TestWorkerSurvivesPanics(t *testing.T) {
	resetJobState()
	alwaysPanic.Store(true)
	q := queue.NewMemoryQueue()
	require.NoError(t, q.Push(&CountJob{}))

	worker := queue.NewWorker(q)
	worker.Tries = 1
	require.NoError(t, worker.Drain())

	failed, err := q.ListFailed()
	require.NoError(t, err)
	require.Len(t, failed, 1)
	assert.Contains(t, failed[0].Exception, "panic")
}

func TestDelayedDispatch(t *testing.T) {
	resetJobState()
	q := queue.NewMemoryQueue()
	require.NoError(t, q.Later(time.Hour, &CountJob{}))

	job, err := q.Pop("")
	require.NoError(t, err)
	assert.Nil(t, job, "delayed job must not be available yet")
}

// dbExecutor adapts *sql.DB to query.Executor for the database driver test.
type dbExecutor struct{ db *sql.DB }

func (e *dbExecutor) Query(q string, b ...any) (*sql.Rows, error) { return e.db.Query(q, b...) }
func (e *dbExecutor) QueryRow(q string, b ...any) *sql.Row        { return e.db.QueryRow(q, b...) }
func (e *dbExecutor) Exec(q string, b ...any) (sql.Result, error) { return e.db.Exec(q, b...) }

func newQueueDB(t *testing.T) *dbExecutor {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	for _, stmt := range []string{
		`CREATE TABLE jobs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			queue TEXT NOT NULL,
			name TEXT NOT NULL,
			payload TEXT NOT NULL,
			attempts INTEGER NOT NULL DEFAULT 0,
			reserved_at INTEGER,
			available_at INTEGER NOT NULL,
			created_at INTEGER NOT NULL
		)`,
		`CREATE TABLE failed_jobs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			queue TEXT NOT NULL,
			name TEXT NOT NULL,
			payload TEXT NOT NULL,
			exception TEXT NOT NULL,
			failed_at INTEGER NOT NULL
		)`,
	} {
		_, err := db.Exec(stmt)
		require.NoError(t, err)
	}
	return &dbExecutor{db: db}
}

func TestDatabaseQueue(t *testing.T) {
	resetJobState()
	q := queue.NewDatabaseQueue("sqlite", newQueueDB(t))

	require.NoError(t, q.Push(&CountJob{Label: "persistent"}))
	size, err := q.Size("")
	require.NoError(t, err)
	assert.EqualValues(t, 1, size)

	worker := queue.NewWorker(q)
	require.NoError(t, worker.Drain())
	assert.EqualValues(t, 1, handled.Load())

	size, err = q.Size("")
	require.NoError(t, err)
	assert.EqualValues(t, 0, size)
}

func TestDatabaseQueueFailureFlow(t *testing.T) {
	resetJobState()
	failUntil.Store(1000)
	q := queue.NewDatabaseQueue("sqlite", newQueueDB(t))
	require.NoError(t, q.Push(&CountJob{Label: "flaky"}))

	worker := queue.NewWorker(q)
	worker.Tries = 2
	require.NoError(t, worker.Drain())
	assert.EqualValues(t, 2, handled.Load())

	failed, err := q.ListFailed()
	require.NoError(t, err)
	require.Len(t, failed, 1)
	assert.Equal(t, "flaky", mustLabel(t, failed[0].Payload))

	failUntil.Store(0)
	require.NoError(t, q.RetryFailed(failed[0].ID))
	require.NoError(t, worker.Drain())
	failed, err = q.ListFailed()
	require.NoError(t, err)
	assert.Empty(t, failed)
}

func mustLabel(t *testing.T, payload []byte) string {
	t.Helper()
	assert.Contains(t, string(payload), "label")
	// The payload envelope is {"job": ..., "data": {"label": ...}}.
	var env struct {
		Data struct {
			Label string `json:"label"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(payload, &env))
	return env.Data.Label
}
