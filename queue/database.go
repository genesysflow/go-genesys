package queue

import (
	"fmt"
	"time"

	"github.com/genesysflow/go-genesys/query"
)

// DatabaseQueue is a durable queue driver backed by a database table.
//
// Expected schema (see queue.CreateJobsTables):
//
//	jobs:        id, queue, name, payload, attempts, reserved_at, available_at, created_at
//	failed_jobs: id, queue, name, payload, exception, failed_at
type DatabaseQueue struct {
	driver      string
	executor    query.Executor
	table       string
	failedTable string
	queue       string
}

// DatabaseQueueConfig configures a DatabaseQueue.
type DatabaseQueueConfig struct {
	// Table is the jobs table name (default "jobs").
	Table string

	// FailedTable is the failed jobs table name (default "failed_jobs").
	FailedTable string

	// Queue is the default queue name for pushed jobs (default "default").
	Queue string
}

// NewDatabaseQueue creates a database queue for the given SQL driver name
// and executor (a contracts.Connection satisfies query.Executor).
func NewDatabaseQueue(driver string, executor query.Executor, config ...DatabaseQueueConfig) *DatabaseQueue {
	cfg := DatabaseQueueConfig{}
	if len(config) > 0 {
		cfg = config[0]
	}
	if cfg.Table == "" {
		cfg.Table = "jobs"
	}
	if cfg.FailedTable == "" {
		cfg.FailedTable = "failed_jobs"
	}
	if cfg.Queue == "" {
		cfg.Queue = "default"
	}
	return &DatabaseQueue{
		driver:      driver,
		executor:    executor,
		table:       cfg.Table,
		failedTable: cfg.FailedTable,
		queue:       cfg.Queue,
	}
}

func (q *DatabaseQueue) jobs() *query.Builder {
	return query.New(q.driver, q.executor).Table(q.table)
}

func (q *DatabaseQueue) failedJobs() *query.Builder {
	return query.New(q.driver, q.executor).Table(q.failedTable)
}

// Push dispatches a job onto the queue.
func (q *DatabaseQueue) Push(job Job) error {
	return q.Later(0, job)
}

// Later dispatches a job to become available after the given delay.
func (q *DatabaseQueue) Later(delay time.Duration, job Job) error {
	name, body, err := marshalJob(job)
	if err != nil {
		return err
	}
	now := time.Now().Unix()
	return q.jobs().Insert(map[string]any{
		"queue":        q.queue,
		"name":         name,
		"payload":      string(body),
		"attempts":     0,
		"reserved_at":  nil,
		"available_at": now + int64(delay/time.Second),
		"created_at":   now,
	})
}

// Pop reserves the next available job. Reservation is claimed with a
// conditional UPDATE so concurrent workers never process the same job.
func (q *DatabaseQueue) Pop(queueName string) (*ReservedJob, error) {
	if queueName == "" {
		queueName = q.queue
	}
	now := time.Now().Unix()

	for i := 0; i < 5; i++ {
		row, err := q.jobs().
			Where("queue", queueName).
			WhereNull("reserved_at").
			Where("available_at", "<=", now).
			OrderBy("id").
			First()
		if err != nil {
			if err == query.ErrNoRows {
				return nil, nil
			}
			return nil, err
		}

		id := toInt64Value(row["id"])
		attempts := int(toInt64Value(row["attempts"])) + 1

		claimed, err := q.jobs().
			Where("id", id).
			WhereNull("reserved_at").
			Update(map[string]any{"reserved_at": now, "attempts": attempts})
		if err != nil {
			return nil, err
		}
		if claimed == 0 {
			continue // another worker claimed it; try the next job
		}

		return &ReservedJob{
			ID:       id,
			Queue:    queueName,
			Name:     fmt.Sprint(row["name"]),
			Payload:  []byte(fmt.Sprint(row["payload"])),
			Attempts: attempts,
		}, nil
	}
	return nil, nil
}

// Delete removes a completed job.
func (q *DatabaseQueue) Delete(job *ReservedJob) error {
	_, err := q.jobs().Where("id", job.ID).Delete()
	return err
}

// Release returns a failed job to the queue after a delay.
func (q *DatabaseQueue) Release(job *ReservedJob, delay time.Duration) error {
	_, err := q.jobs().Where("id", job.ID).Update(map[string]any{
		"reserved_at":  nil,
		"available_at": time.Now().Add(delay).Unix(),
	})
	return err
}

// Fail moves a job to the failed jobs table.
func (q *DatabaseQueue) Fail(job *ReservedJob, jobErr error) error {
	if err := q.failedJobs().Insert(map[string]any{
		"queue":     job.Queue,
		"name":      job.Name,
		"payload":   string(job.Payload),
		"exception": jobErr.Error(),
		"failed_at": time.Now().Unix(),
	}); err != nil {
		return err
	}
	return q.Delete(job)
}

// ListFailed returns all failed jobs, newest first.
func (q *DatabaseQueue) ListFailed() ([]FailedJob, error) {
	rows, err := q.failedJobs().OrderByDesc("id").Get()
	if err != nil {
		return nil, err
	}
	failed := make([]FailedJob, 0, len(rows))
	for _, row := range rows {
		failed = append(failed, FailedJob{
			ID:        toInt64Value(row["id"]),
			Queue:     fmt.Sprint(row["queue"]),
			Name:      fmt.Sprint(row["name"]),
			Payload:   []byte(fmt.Sprint(row["payload"])),
			Exception: fmt.Sprint(row["exception"]),
			FailedAt:  time.Unix(toInt64Value(row["failed_at"]), 0),
		})
	}
	return failed, nil
}

// RetryFailed re-dispatches a failed job and removes it from the failed table.
func (q *DatabaseQueue) RetryFailed(id int64) error {
	row, err := q.failedJobs().Where("id", id).First()
	if err != nil {
		if err == query.ErrNoRows {
			return fmt.Errorf("queue: failed job [%d] not found", id)
		}
		return err
	}

	now := time.Now().Unix()
	if err := q.jobs().Insert(map[string]any{
		"queue":        fmt.Sprint(row["queue"]),
		"name":         fmt.Sprint(row["name"]),
		"payload":      fmt.Sprint(row["payload"]),
		"attempts":     0,
		"reserved_at":  nil,
		"available_at": now,
		"created_at":   now,
	}); err != nil {
		return err
	}
	_, err = q.failedJobs().Where("id", id).Delete()
	return err
}

// ForgetFailed removes a failed job without retrying it.
func (q *DatabaseQueue) ForgetFailed(id int64) error {
	_, err := q.failedJobs().Where("id", id).Delete()
	return err
}

// Size returns the number of pending jobs on a queue.
func (q *DatabaseQueue) Size(queueName string) (int64, error) {
	if queueName == "" {
		queueName = q.queue
	}
	return q.jobs().Where("queue", queueName).WhereNull("reserved_at").Count()
}

func toInt64Value(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case int:
		return int64(n)
	case float64:
		return int64(n)
	case string:
		var out int64
		fmt.Sscanf(n, "%d", &out)
		return out
	case []byte:
		var out int64
		fmt.Sscanf(string(n), "%d", &out)
		return out
	}
	return 0
}
