package queue_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/genesysflow/go-genesys/queue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type slowJob struct{ Naptime int64 }

func (j *slowJob) Handle() error {
	time.Sleep(time.Duration(j.Naptime) * time.Millisecond)
	return nil
}

type ctxAwareJob struct{ Naptime int64 }

var ctxJobCancelled atomic.Bool

func (j *ctxAwareJob) Handle() error { return j.HandleContext(context.Background()) }
func (j *ctxAwareJob) HandleContext(ctx context.Context) error {
	select {
	case <-time.After(time.Duration(j.Naptime) * time.Millisecond):
		return nil
	case <-ctx.Done():
		ctxJobCancelled.Store(true)
		return ctx.Err()
	}
}

func TestJobTimeoutFailsTheAttempt(t *testing.T) {
	queue.Register[slowJob]()
	q := queue.NewMemoryQueue()
	require.NoError(t, q.Push(&slowJob{Naptime: 300}))

	worker := queue.NewWorker(q)
	worker.Tries = 1
	worker.Timeout = 30 * time.Millisecond
	require.NoError(t, worker.Drain())

	failed, err := q.ListFailed()
	require.NoError(t, err)
	require.Len(t, failed, 1)
	assert.Contains(t, failed[0].Exception, "timed out")
}

func TestContextJobSeesTheDeadline(t *testing.T) {
	queue.Register[ctxAwareJob]()
	ctxJobCancelled.Store(false)
	q := queue.NewMemoryQueue()
	require.NoError(t, q.Push(&ctxAwareJob{Naptime: 300}))

	worker := queue.NewWorker(q)
	worker.Tries = 1
	worker.Timeout = 30 * time.Millisecond
	require.NoError(t, worker.Drain())

	// Give the goroutine a beat to observe cancellation.
	require.Eventually(t, ctxJobCancelled.Load, time.Second, 10*time.Millisecond,
		"a ContextJob must receive the deadline through its context")
}

func TestFastJobUnaffectedByTimeout(t *testing.T) {
	queue.Register[slowJob]()
	q := queue.NewMemoryQueue()
	require.NoError(t, q.Push(&slowJob{Naptime: 1}))

	worker := queue.NewWorker(q)
	worker.Timeout = time.Second
	require.NoError(t, worker.Drain())
	failed, err := q.ListFailed()
	require.NoError(t, err)
	assert.Empty(t, failed)
}

func TestPruneFailed(t *testing.T) {
	q := queue.NewMemoryQueue()
	require.NoError(t, q.Push(&slowJob{}))
	job, err := q.Pop("")
	require.NoError(t, err)
	require.NoError(t, q.Fail(job, errors.New("boom")))

	// Nothing is old enough yet.
	pruned, err := q.PruneFailed(time.Now().Add(-time.Hour))
	require.NoError(t, err)
	assert.EqualValues(t, 0, pruned)

	// Everything older than the future cutoff goes.
	pruned, err = q.PruneFailed(time.Now().Add(time.Hour))
	require.NoError(t, err)
	assert.EqualValues(t, 1, pruned)
	failed, err := q.ListFailed()
	require.NoError(t, err)
	assert.Empty(t, failed)
}
