// Package schedule provides Laravel-style task scheduling. Define the
// schedule once, then drive it with `genesys schedule:run` from system cron
// (every minute) or keep `genesys schedule:work` running.
package schedule

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Event is a scheduled task.
type Event struct {
	spec        *cronSpec
	expr        string
	description string
	run         func() error

	overlapping bool // allow overlapping runs (default: prevented)
	running     sync.Mutex
	err         error // deferred parse error, surfaced on run
}

// Schedule holds the application's scheduled events.
type Schedule struct {
	events []*Event
	mu     sync.Mutex
}

// New creates an empty schedule.
func New() *Schedule {
	return &Schedule{}
}

// Call schedules a function. Chain frequency methods to set when it runs:
//
//	schedule.Call(cleanup).Daily()
//	schedule.Call(report).Cron("0 8 * * 1-5")
func (s *Schedule) Call(fn func() error) *Event {
	event := &Event{run: fn}
	event.Cron("* * * * *")
	s.mu.Lock()
	s.events = append(s.events, event)
	s.mu.Unlock()
	return event
}

// Events returns all scheduled events.
func (s *Schedule) Events() []*Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]*Event(nil), s.events...)
}

// Description labels the event for logs and schedule listings.
func (e *Event) Description(text string) *Event {
	e.description = text
	return e
}

// GetDescription returns the event's label.
func (e *Event) GetDescription() string {
	return e.description
}

// Expression returns the event's cron expression.
func (e *Event) Expression() string {
	return e.expr
}

// Cron schedules the event with a raw 5-field cron expression.
func (e *Event) Cron(expr string) *Event {
	spec, err := parseCron(expr)
	if err != nil {
		e.err = err
		return e
	}
	e.spec = spec
	e.expr = expr
	return e
}

// EveryMinute runs the event every minute.
func (e *Event) EveryMinute() *Event { return e.Cron("* * * * *") }

// EveryFiveMinutes runs the event every five minutes.
func (e *Event) EveryFiveMinutes() *Event { return e.Cron("*/5 * * * *") }

// EveryTenMinutes runs the event every ten minutes.
func (e *Event) EveryTenMinutes() *Event { return e.Cron("*/10 * * * *") }

// EveryThirtyMinutes runs the event every thirty minutes.
func (e *Event) EveryThirtyMinutes() *Event { return e.Cron("*/30 * * * *") }

// Hourly runs the event at the top of every hour.
func (e *Event) Hourly() *Event { return e.Cron("0 * * * *") }

// HourlyAt runs the event at the given minute of every hour.
func (e *Event) HourlyAt(minute int) *Event {
	return e.Cron(fmt.Sprintf("%d * * * *", minute))
}

// Daily runs the event at midnight.
func (e *Event) Daily() *Event { return e.Cron("0 0 * * *") }

// DailyAt runs the event daily at the given "15:04" time.
func (e *Event) DailyAt(at string) *Event {
	t, err := time.Parse("15:04", at)
	if err != nil {
		e.err = fmt.Errorf("schedule: DailyAt expects HH:MM, got %q", at)
		return e
	}
	return e.Cron(fmt.Sprintf("%d %d * * *", t.Minute(), t.Hour()))
}

// Weekly runs the event on Sundays at midnight.
func (e *Event) Weekly() *Event { return e.Cron("0 0 * * 0") }

// WeeklyOn runs the event weekly on the given day (0 = Sunday) and time.
func (e *Event) WeeklyOn(day int, at string) *Event {
	t, err := time.Parse("15:04", at)
	if err != nil {
		e.err = fmt.Errorf("schedule: WeeklyOn expects HH:MM, got %q", at)
		return e
	}
	return e.Cron(fmt.Sprintf("%d %d * * %d", t.Minute(), t.Hour(), day))
}

// Monthly runs the event on the first of the month at midnight.
func (e *Event) Monthly() *Event { return e.Cron("0 0 1 * *") }

// AllowOverlapping lets a run start even when the previous run has not
// finished (prevented by default).
func (e *Event) AllowOverlapping() *Event {
	e.overlapping = true
	return e
}

// IsDue reports whether the event should run at the given time.
func (e *Event) IsDue(t time.Time) bool {
	return e.err == nil && e.spec != nil && e.spec.matches(t)
}

// Run executes the event.
func (e *Event) Run() error {
	if e.err != nil {
		return e.err
	}
	if !e.overlapping {
		if !e.running.TryLock() {
			return nil // previous run still in progress
		}
		defer e.running.Unlock()
	}
	return safeRun(e.run)
}

func safeRun(fn func() error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("schedule: task panicked: %v", r)
		}
	}()
	return fn()
}

// RunDue runs every event due at the given time, collecting errors keyed
// by description (or cron expression).
func (s *Schedule) RunDue(t time.Time) map[string]error {
	results := make(map[string]error)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, event := range s.Events() {
		if !event.IsDue(t) {
			continue
		}
		wg.Add(1)
		go func(e *Event) {
			defer wg.Done()
			err := e.Run()
			mu.Lock()
			results[e.label()] = err
			mu.Unlock()
		}(event)
	}
	wg.Wait()
	return results
}

func (e *Event) label() string {
	if e.description != "" {
		return e.description
	}
	return e.expr
}

// Work runs the schedule until the context is cancelled, evaluating due
// events at the top of every minute.
func (s *Schedule) Work(ctx context.Context, onResult func(label string, err error)) error {
	for {
		now := time.Now()
		next := now.Truncate(time.Minute).Add(time.Minute)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Until(next)):
		}

		for label, err := range s.RunDue(next) {
			if onResult != nil {
				onResult(label, err)
			}
		}
	}
}
