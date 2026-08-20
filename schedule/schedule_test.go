package schedule_test

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/genesysflow/go-genesys/schedule"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func at(day time.Weekday, hour, minute int) time.Time {
	// 2026-08-16 is a Sunday.
	base := time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC)
	return base.AddDate(0, 0, int(day)).Add(time.Duration(hour)*time.Hour + time.Duration(minute)*time.Minute)
}

func TestFrequencyHelpers(t *testing.T) {
	s := schedule.New()
	noop := func() error { return nil }

	tests := []struct {
		event *schedule.Event
		due   []time.Time
		not   []time.Time
	}{
		{
			event: s.Call(noop).EveryMinute(),
			due:   []time.Time{at(time.Monday, 10, 31)},
		},
		{
			event: s.Call(noop).EveryFiveMinutes(),
			due:   []time.Time{at(time.Monday, 10, 30), at(time.Monday, 10, 35)},
			not:   []time.Time{at(time.Monday, 10, 31)},
		},
		{
			event: s.Call(noop).Hourly(),
			due:   []time.Time{at(time.Monday, 14, 0)},
			not:   []time.Time{at(time.Monday, 14, 30)},
		},
		{
			event: s.Call(noop).DailyAt("08:30"),
			due:   []time.Time{at(time.Tuesday, 8, 30)},
			not:   []time.Time{at(time.Tuesday, 9, 30), at(time.Tuesday, 8, 31)},
		},
		{
			event: s.Call(noop).WeeklyOn(1, "09:00"),
			due:   []time.Time{at(time.Monday, 9, 0)},
			not:   []time.Time{at(time.Tuesday, 9, 0)},
		},
		{
			event: s.Call(noop).Cron("*/15 8-17 * * 1-5"),
			due:   []time.Time{at(time.Friday, 8, 0), at(time.Wednesday, 17, 45)},
			not:   []time.Time{at(time.Sunday, 8, 0), at(time.Wednesday, 18, 0), at(time.Wednesday, 8, 7)},
		},
	}

	for _, tc := range tests {
		for _, ts := range tc.due {
			assert.True(t, tc.event.IsDue(ts), "%s should be due at %s", tc.event.Expression(), ts)
		}
		for _, ts := range tc.not {
			assert.False(t, tc.event.IsDue(ts), "%s should not be due at %s", tc.event.Expression(), ts)
		}
	}
}

func TestInvalidCron(t *testing.T) {
	s := schedule.New()
	event := s.Call(func() error { return nil }).Cron("not a cron")
	assert.False(t, event.IsDue(time.Now()))
	assert.Error(t, event.Run())
}

func TestRunDue(t *testing.T) {
	s := schedule.New()
	var ran atomic.Int64

	s.Call(func() error { ran.Add(1); return nil }).EveryMinute().Description("counter")
	s.Call(func() error { return errors.New("boom") }).EveryMinute().Description("failing")
	s.Call(func() error { ran.Add(100); return nil }).Daily().Description("midnight-only")

	results := s.RunDue(at(time.Monday, 10, 30))
	require.Len(t, results, 2)
	assert.NoError(t, results["counter"])
	assert.EqualValues(t, 1, ran.Load())
	assert.ErrorContains(t, results["failing"], "boom")
}

func TestPanicRecovered(t *testing.T) {
	s := schedule.New()
	s.Call(func() error { panic("kaboom") }).EveryMinute().Description("panicky")

	results := s.RunDue(at(time.Monday, 10, 30))
	assert.ErrorContains(t, results["panicky"], "kaboom")
}

func TestOverlapPrevention(t *testing.T) {
	s := schedule.New()
	release := make(chan struct{})
	var runs atomic.Int64

	event := s.Call(func() error {
		runs.Add(1)
		<-release
		return nil
	}).EveryMinute()

	go event.Run()
	// Give the first run a moment to acquire the lock.
	time.Sleep(20 * time.Millisecond)
	require.NoError(t, event.Run()) // skipped silently, no second run
	close(release)
	time.Sleep(20 * time.Millisecond)
	assert.EqualValues(t, 1, runs.Load())
}
