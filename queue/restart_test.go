package queue_test

import (
	"context"
	"testing"
	"time"

	"github.com/genesysflow/go-genesys/cache"
	"github.com/genesysflow/go-genesys/queue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type countingJob struct {
	Label string `json:"label"`
}

func (j *countingJob) JobName() string { return "counting" }
func (j *countingJob) Handle() error   { return nil }

func TestWorkerStopsOnRestartSignal(t *testing.T) {
	queue.Register[countingJob]()

	store := cache.NewMemoryStore()
	driver := queue.NewMemoryQueue()

	worker := queue.NewWorker(driver)
	worker.Sleep = time.Millisecond
	worker.WatchRestart(store)

	require.NoError(t, driver.Push(&countingJob{Label: "one"}))

	// A restart issued before the worker starts stops it once the queue
	// drains, rather than mid-job.
	require.NoError(t, queue.SignalRestart(store))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := worker.Run(ctx)
	require.ErrorIs(t, err, queue.ErrRestartRequested)
	assert.NotErrorIs(t, err, context.DeadlineExceeded, "the worker should stop on the signal, not the deadline")
}

// A worker started after the signal was issued must not stop: the
// signal applies to workers running when it was sent.
func TestRestartSignalOnlyStopsOlderWorkers(t *testing.T) {
	store := cache.NewMemoryStore()
	require.NoError(t, queue.SignalRestart(store))

	// Give the timestamp a moment so the worker's start time is later.
	time.Sleep(10 * time.Millisecond)

	driver := queue.NewMemoryQueue()
	worker := queue.NewWorker(driver)
	worker.Sleep = time.Millisecond
	worker.WatchRestart(store)

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	err := worker.Run(ctx)
	assert.ErrorIs(t, err, context.DeadlineExceeded, "a freshly started worker ignores an older signal")
}

// Without a store to watch, the worker runs as before.
func TestWorkerWithoutRestartWatch(t *testing.T) {
	driver := queue.NewMemoryQueue()
	worker := queue.NewWorker(driver)
	worker.Sleep = time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	assert.ErrorIs(t, worker.Run(ctx), context.DeadlineExceeded)
}

// --- sizes -----------------------------------------------------------

func TestMemoryQueueReportsItsSize(t *testing.T) {
	driver := queue.NewMemoryQueue()

	sizer, ok := any(driver).(queue.SizeProvider)
	require.True(t, ok, "the memory driver should report its size")

	require.NoError(t, driver.Push(&countingJob{Label: "one"}))
	require.NoError(t, driver.Push(&countingJob{Label: "two"}))

	size, err := sizer.Size("")
	require.NoError(t, err)
	assert.Equal(t, int64(2), size)

	// Popping a job takes it out of the count.
	reserved, err := driver.Pop("")
	require.NoError(t, err)
	require.NotNil(t, reserved)

	size, err = sizer.Size("")
	require.NoError(t, err)
	assert.Equal(t, int64(1), size)
}
