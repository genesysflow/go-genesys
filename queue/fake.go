package queue

import (
	"reflect"
	"sync"
	"time"
)

// TestingT is the subset of *testing.T the assertion helpers need.
type TestingT interface {
	Helper()
	Errorf(format string, args ...any)
}

// PushedJob records one dispatch captured by a Fake.
type PushedJob struct {
	Job   Job
	Name  string
	Delay time.Duration
}

// Fake is a queue driver that records dispatched jobs without running
// them - Laravel's Queue::fake. Swap it in via the manager or facade,
// run the code under test, then assert:
//
//	fake := queue.NewFake()
//	manager.Register("sync", fake)
//	...
//	queue.AssertPushed[SendEmailJob](t, fake)
type Fake struct {
	mu     sync.Mutex
	pushed []PushedJob
}

// NewFake creates a recording queue fake.
func NewFake() *Fake {
	return &Fake{}
}

// Push records a job without executing it.
func (f *Fake) Push(job Job) error {
	return f.Later(0, job)
}

// Later records a delayed job without executing it.
func (f *Fake) Later(delay time.Duration, job Job) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pushed = append(f.pushed, PushedJob{Job: job, Name: nameForJob(job), Delay: delay})
	return nil
}

// Pushed returns everything dispatched so far.
func (f *Fake) Pushed() []PushedJob {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]PushedJob(nil), f.pushed...)
}

// Clear forgets all recorded jobs.
func (f *Fake) Clear() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pushed = nil
}

// AssertPushedCount asserts the total number of dispatched jobs.
func (f *Fake) AssertPushedCount(t TestingT, expected int) {
	t.Helper()
	if got := len(f.Pushed()); got != expected {
		t.Errorf("expected %d pushed jobs, got %d", expected, got)
	}
}

// AssertNothingPushed asserts no jobs were dispatched.
func (f *Fake) AssertNothingPushed(t TestingT) {
	t.Helper()
	if pushed := f.Pushed(); len(pushed) > 0 {
		t.Errorf("expected no pushed jobs, got %d (first: %s)", len(pushed), pushed[0].Name)
	}
}

// AssertPushed asserts at least one job of type T was dispatched,
// optionally matching a predicate, and returns the matches.
func AssertPushed[T any](t TestingT, fake *Fake, match ...func(*T) bool) []*T {
	t.Helper()
	var matches []*T
	for _, pushed := range fake.Pushed() {
		job, ok := any(pushed.Job).(*T)
		if !ok {
			continue
		}
		if len(match) > 0 && !match[0](job) {
			continue
		}
		matches = append(matches, job)
	}
	if len(matches) == 0 {
		t.Errorf("expected a pushed %s job", reflect.TypeOf((*T)(nil)).Elem().Name())
	}
	return matches
}

// AssertNotPushed asserts no job of type T was dispatched.
func AssertNotPushed[T any](t TestingT, fake *Fake) {
	t.Helper()
	for _, pushed := range fake.Pushed() {
		if _, ok := any(pushed.Job).(*T); ok {
			t.Errorf("expected no pushed %s job, found one", reflect.TypeOf((*T)(nil)).Elem().Name())
			return
		}
	}
}
