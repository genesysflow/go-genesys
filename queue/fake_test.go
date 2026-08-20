package queue_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/genesysflow/go-genesys/queue"
	"github.com/stretchr/testify/assert"
)

type welcomeEmailJob struct {
	Email string
}

func (j *welcomeEmailJob) Handle() error {
	panic("fake queues must never run jobs")
}

type reportJob struct{}

func (j *reportJob) Handle() error { return nil }

type fakeT struct{ failures []string }

func (f *fakeT) Helper() {}
func (f *fakeT) Errorf(format string, args ...any) {
	f.failures = append(f.failures, fmt.Sprintf(format, args...))
}

func TestQueueFakeRecordsWithoutRunning(t *testing.T) {
	fake := queue.NewFake()

	assert.NoError(t, fake.Push(&welcomeEmailJob{Email: "a@x.io"}))
	assert.NoError(t, fake.Later(time.Hour, &welcomeEmailJob{Email: "b@x.io"}))

	pushed := fake.Pushed()
	assert.Len(t, pushed, 2)
	assert.Equal(t, time.Hour, pushed[1].Delay)

	fake.AssertPushedCount(t, 2)
	matches := queue.AssertPushed[welcomeEmailJob](t, fake)
	assert.Len(t, matches, 2)
	queue.AssertPushed[welcomeEmailJob](t, fake, func(j *welcomeEmailJob) bool {
		return j.Email == "b@x.io"
	})
	queue.AssertNotPushed[reportJob](t, fake)

	fake.Clear()
	fake.AssertNothingPushed(t)
}

func TestQueueFakeAssertionFailures(t *testing.T) {
	fake := queue.NewFake()
	rec := &fakeT{}

	fake.AssertPushedCount(rec, 1)
	queue.AssertPushed[reportJob](rec, fake)

	assert.NoError(t, fake.Push(&reportJob{}))
	queue.AssertNotPushed[reportJob](rec, fake)
	fake.AssertNothingPushed(rec)

	assert.Len(t, rec.failures, 4)
}
