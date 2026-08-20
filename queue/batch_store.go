package queue

import (
	"fmt"
	"sync"
	"time"

	"github.com/genesysflow/go-genesys/query"
)

// BatchRepository persists batch progress so it survives the process
// and is visible to workers in other processes - Laravel's job_batches
// table. Completion callbacks still fire only in the process that
// registered them; progress and counts are shared.
type BatchRepository interface {
	// Insert records a new batch with its job total.
	Insert(id string, total int) error

	// Add increases a stored batch's job total (jobs dispatched later).
	Add(id string, jobs int) error

	// Increment records one settled job and returns the new counters.
	Increment(id string, failed bool) (BatchProgress, error)

	// Find returns a batch's counters.
	Find(id string) (BatchProgress, error)

	// Delete removes a finished batch's record.
	Delete(id string) error
}

// BatchProgress is a batch's persisted counters.
type BatchProgress struct {
	ID        string `json:"id"`
	Total     int    `json:"total"`
	Completed int    `json:"completed"`
	Failed    int    `json:"failed"`
}

// Finished reports whether every job has settled.
func (p BatchProgress) Finished() bool {
	return p.Total > 0 && p.Completed+p.Failed >= p.Total
}

var (
	batchRepoMu sync.RWMutex
	batchRepo   BatchRepository
)

// SetBatchRepository installs a shared repository used by every batch
// (pass nil to disable). Workers in any process then persist progress:
//
//	queue.SetBatchRepository(queue.NewDatabaseBatchRepository(conn.Driver(), conn, ""))
func SetBatchRepository(repo BatchRepository) {
	batchRepoMu.Lock()
	defer batchRepoMu.Unlock()
	batchRepo = repo
}

func currentBatchRepository() BatchRepository {
	batchRepoMu.RLock()
	defer batchRepoMu.RUnlock()
	return batchRepo
}

// FindBatch returns a batch's persisted progress from any process.
func FindBatch(id string) (BatchProgress, error) {
	repo := currentBatchRepository()
	if repo == nil {
		return BatchProgress{}, fmt.Errorf("queue: no batch repository configured (call SetBatchRepository)")
	}
	return repo.Find(id)
}

// DatabaseBatchRepository stores batch counters in a table:
//
//	CREATE TABLE job_batches (
//	    id             TEXT PRIMARY KEY,
//	    total_jobs     INTEGER NOT NULL,
//	    completed_jobs INTEGER NOT NULL DEFAULT 0,
//	    failed_jobs    INTEGER NOT NULL DEFAULT 0,
//	    created_at     TIMESTAMP NOT NULL
//	);
type DatabaseBatchRepository struct {
	driver   string
	executor query.Executor
	table    string
}

// NewDatabaseBatchRepository creates a repository over the given
// connection. An empty table name defaults to "job_batches".
func NewDatabaseBatchRepository(driver string, executor query.Executor, table string) *DatabaseBatchRepository {
	if table == "" {
		table = "job_batches"
	}
	return &DatabaseBatchRepository{driver: driver, executor: executor, table: table}
}

func (r *DatabaseBatchRepository) query() *query.Builder {
	return query.New(r.driver, r.executor).Table(r.table)
}

// Insert records a new batch.
func (r *DatabaseBatchRepository) Insert(id string, total int) error {
	return r.query().Insert(map[string]any{
		"id":             id,
		"total_jobs":     total,
		"completed_jobs": 0,
		"failed_jobs":    0,
		"created_at":     time.Now().UTC(),
	})
}

// Add increases a batch's job total.
func (r *DatabaseBatchRepository) Add(id string, jobs int) error {
	_, err := r.query().Where("id", id).Increment("total_jobs", jobs)
	return err
}

// Increment records one settled job atomically and returns the new counters.
func (r *DatabaseBatchRepository) Increment(id string, failed bool) (BatchProgress, error) {
	column := "completed_jobs"
	if failed {
		column = "failed_jobs"
	}
	if _, err := r.query().Where("id", id).Increment(column, 1); err != nil {
		return BatchProgress{}, err
	}
	return r.Find(id)
}

// Find returns a batch's counters.
func (r *DatabaseBatchRepository) Find(id string) (BatchProgress, error) {
	rows, err := r.query().Where("id", id).Get()
	if err != nil {
		return BatchProgress{}, err
	}
	if len(rows) == 0 {
		return BatchProgress{}, fmt.Errorf("queue: batch %s not found", id)
	}
	row := rows[0]
	progress := BatchProgress{ID: id}
	progress.Total = intFrom(row["total_jobs"])
	progress.Completed = intFrom(row["completed_jobs"])
	progress.Failed = intFrom(row["failed_jobs"])
	return progress, nil
}

// Delete removes a batch's record.
func (r *DatabaseBatchRepository) Delete(id string) error {
	_, err := r.query().Where("id", id).Delete()
	return err
}

func intFrom(value any) int {
	switch v := value.(type) {
	case int64:
		return int(v)
	case int:
		return v
	case float64:
		return int(v)
	}
	return 0
}
