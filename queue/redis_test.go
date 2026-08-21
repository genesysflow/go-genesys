package queue_test

import (
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/genesysflow/go-genesys/queue"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type redisTestJob struct {
	Word string `json:"word"`
}

var redisJobRuns []string

func (j *redisTestJob) Handle() error {
	redisJobRuns = append(redisJobRuns, j.Word)
	if j.Word == "fail" {
		return errors.New("job exploded")
	}
	return nil
}

func newRedisQueue(t *testing.T) (*queue.RedisQueue, *miniredis.Miniredis) {
	t.Helper()
	queue.Register[redisTestJob]()
	redisJobRuns = nil
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	q := queue.NewRedisQueueWithClient(client, "")
	t.Cleanup(func() { client.Close() })
	return q, mr
}

func TestRedisQueuePushPopDelete(t *testing.T) {
	q, _ := newRedisQueue(t)

	require.NoError(t, q.Push(&redisTestJob{Word: "one"}))
	require.NoError(t, q.Push(&redisTestJob{Word: "two"}))
	assert.Equal(t, 2, queueSize(t, q, ""))

	job, err := q.Pop("")
	require.NoError(t, err)
	require.NotNil(t, job)
	assert.Equal(t, 1, job.Attempts)
	assert.Equal(t, "default", job.Queue)
	require.NoError(t, q.Delete(job))

	// FIFO order.
	second, err := q.Pop("")
	require.NoError(t, err)
	require.NotNil(t, second)
	assert.Greater(t, second.ID, job.ID)

	empty, err := q.Pop("")
	require.NoError(t, err)
	assert.Nil(t, empty)
}

func TestRedisQueueDelayedJobs(t *testing.T) {
	q, _ := newRedisQueue(t)

	// The due-check compares against wall-clock time, so use a real
	// (tiny) delay rather than miniredis's fake clock.
	require.NoError(t, q.Later(50*time.Millisecond, &redisTestJob{Word: "later"}))
	assert.Equal(t, 1, queueSize(t, q, ""), "delayed jobs count toward size")

	job, err := q.Pop("")
	require.NoError(t, err)
	assert.Nil(t, job, "not due yet")

	deadline := time.Now().Add(2 * time.Second)
	for job == nil && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
		job, err = q.Pop("")
		require.NoError(t, err)
	}
	require.NotNil(t, job, "due after the delay")
}

func TestRedisQueueReleaseKeepsAttempts(t *testing.T) {
	q, _ := newRedisQueue(t)

	require.NoError(t, q.Push(&redisTestJob{Word: "retry"}))
	job, err := q.Pop("")
	require.NoError(t, err)
	require.NotNil(t, job)
	assert.Equal(t, 1, job.Attempts)

	require.NoError(t, q.Release(job, 0))
	again, err := q.Pop("")
	require.NoError(t, err)
	require.NotNil(t, again)
	assert.Equal(t, 2, again.Attempts, "attempts survive a release")
}

func TestRedisQueueFailedLifecycle(t *testing.T) {
	q, _ := newRedisQueue(t)

	require.NoError(t, q.Push(&redisTestJob{Word: "fail"}))
	job, err := q.Pop("")
	require.NoError(t, err)
	require.NotNil(t, job)
	require.NoError(t, q.Fail(job, errors.New("job exploded")))

	failed, err := q.ListFailed()
	require.NoError(t, err)
	require.Len(t, failed, 1)
	assert.Equal(t, "job exploded", failed[0].Exception)
	assert.Equal(t, job.ID, failed[0].ID)

	// Retry puts it back on its queue and clears the failed list.
	require.NoError(t, q.RetryFailed(job.ID))
	failed, err = q.ListFailed()
	require.NoError(t, err)
	assert.Empty(t, failed)
	assert.Equal(t, 1, queueSize(t, q, ""))

	// Forget drops without retrying.
	job2, err := q.Pop("")
	require.NoError(t, err)
	require.NoError(t, q.Fail(job2, errors.New("again")))
	require.NoError(t, q.ForgetFailed(job2.ID))
	failed, err = q.ListFailed()
	require.NoError(t, err)
	assert.Empty(t, failed)

	assert.Error(t, q.RetryFailed(9999), "unknown ids error")
}

func TestRedisQueueWithWorker(t *testing.T) {
	q, _ := newRedisQueue(t)

	require.NoError(t, q.Push(&redisTestJob{Word: "a"}))
	require.NoError(t, q.Push(&redisTestJob{Word: "b"}))

	worker := queue.NewWorker(q)
	require.NoError(t, worker.Drain())
	assert.Equal(t, []string{"a", "b"}, redisJobRuns)
	assert.Equal(t, 0, queueSize(t, q, ""))
}
