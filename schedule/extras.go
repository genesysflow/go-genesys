package schedule

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/genesysflow/go-genesys/cache"
	"github.com/genesysflow/go-genesys/queue"
)

// --- frequencies -----------------------------------------------------

// EveryFifteenMinutes runs the event at :00, :15, :30 and :45.
func (e *Event) EveryFifteenMinutes() *Event { return e.Cron("*/15 * * * *") }

// Weekdays runs the event at midnight, Monday to Friday.
func (e *Event) Weekdays() *Event { return e.Cron("0 0 * * 1-5") }

// Weekends runs the event at midnight on Saturday and Sunday.
func (e *Event) Weekends() *Event { return e.Cron("0 0 * * 6,0") }

// Mondays runs the event at midnight on Mondays. The other weekday
// helpers follow the same shape.
func (e *Event) Mondays() *Event { return e.onDay(1) }

// Tuesdays runs the event at midnight on Tuesdays.
func (e *Event) Tuesdays() *Event { return e.onDay(2) }

// Wednesdays runs the event at midnight on Wednesdays.
func (e *Event) Wednesdays() *Event { return e.onDay(3) }

// Thursdays runs the event at midnight on Thursdays.
func (e *Event) Thursdays() *Event { return e.onDay(4) }

// Fridays runs the event at midnight on Fridays.
func (e *Event) Fridays() *Event { return e.onDay(5) }

// Saturdays runs the event at midnight on Saturdays.
func (e *Event) Saturdays() *Event { return e.onDay(6) }

// Sundays runs the event at midnight on Sundays.
func (e *Event) Sundays() *Event { return e.onDay(0) }

func (e *Event) onDay(day int) *Event {
	return e.Cron(fmt.Sprintf("0 0 * * %d", day))
}

// Days runs the event at midnight on the given weekdays (0 = Sunday).
func (e *Event) Days(days ...int) *Event {
	if len(days) == 0 {
		return e
	}
	rendered := make([]string, 0, len(days))
	for _, day := range days {
		rendered = append(rendered, fmt.Sprint(day))
	}
	return e.Cron("0 0 * * " + strings.Join(rendered, ","))
}

// TwiceDaily runs the event at two hours of the day.
func (e *Event) TwiceDaily(first, second int) *Event {
	return e.Cron(fmt.Sprintf("0 %d,%d * * *", first, second))
}

// Quarterly runs the event at midnight on the first day of each quarter.
func (e *Event) Quarterly() *Event { return e.Cron("0 0 1 1,4,7,10 *") }

// Yearly runs the event at midnight on the first of January.
func (e *Event) Yearly() *Event { return e.Cron("0 0 1 1 *") }

// --- timezone --------------------------------------------------------

// Timezone evaluates the event's schedule in the given location, so
// "daily at 09:00" means 09:00 there rather than 09:00 UTC. An unknown
// name is reported when the event runs.
func (e *Event) Timezone(name string) *Event {
	location, err := time.LoadLocation(name)
	if err != nil {
		e.err = fmt.Errorf("schedule: unknown timezone %q: %w", name, err)
		return e
	}
	e.location = location
	return e
}

// In evaluates the event's schedule in the given location.
func (e *Event) In(location *time.Location) *Event {
	e.location = location
	return e
}

// --- constraints -----------------------------------------------------

// When runs the event only while the condition holds. It is checked when
// the event comes due, not when it is defined.
func (e *Event) When(condition func() bool) *Event {
	e.filters = append(e.filters, condition)
	return e
}

// Skip is the inverse of When: the event is skipped while the condition
// holds.
func (e *Event) Skip(condition func() bool) *Event {
	e.rejects = append(e.rejects, condition)
	return e
}

// Between runs the event only between two times of day ("09:00",
// "17:00"). A window whose end is before its start wraps midnight, which
// is how a night-time maintenance window is expressed.
func (e *Event) Between(start, end string) *Event {
	e.timeFilter(start, end, true)
	return e
}

// UnlessBetween skips the event between two times of day.
func (e *Event) UnlessBetween(start, end string) *Event {
	e.timeFilter(start, end, false)
	return e
}

// timeFilter installs a time-of-day window filter.
func (e *Event) timeFilter(start, end string, inside bool) {
	from, err := parseClock(start)
	if err != nil {
		e.err = err
		return
	}
	to, err := parseClock(end)
	if err != nil {
		e.err = err
		return
	}

	e.timeFilters = append(e.timeFilters, func(at time.Time) bool {
		minutes := at.Hour()*60 + at.Minute()
		within := from <= minutes && minutes <= to
		if to < from {
			// The window wraps midnight.
			within = minutes >= from || minutes <= to
		}
		return within == inside
	})
}

// parseClock parses "HH:MM" into minutes since midnight.
func parseClock(value string) (int, error) {
	parsed, err := time.Parse("15:04", value)
	if err != nil {
		return 0, fmt.Errorf("schedule: invalid time of day %q: %w", value, err)
	}
	return parsed.Hour()*60 + parsed.Minute(), nil
}

// Environments runs the event only in the named environments. The
// schedule's environment comes from SetEnvironment.
func (e *Event) Environments(names ...string) *Event {
	e.environments = append(e.environments, names...)
	return e
}

// SetEnvironment tells the schedule which environment it runs in, for
// events constrained with Environments.
func (s *Schedule) SetEnvironment(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.environment = name
	for _, event := range s.events {
		event.environment = name
	}
}

// --- exec and jobs ---------------------------------------------------

// Exec schedules an external command:
//
//	schedule.Exec("pg_dump", "-Fc", "app").DailyAt("03:00")
//
// Its combined output is kept for LastOutput and can be appended to a
// file with AppendOutputTo; a non-zero exit status is an error.
func (s *Schedule) Exec(name string, args ...string) *Event {
	event := s.Call(nil)
	event.run = func() error { return event.runExec(name, args) }
	event.Description(strings.TrimSpace(name + " " + strings.Join(args, " ")))
	return event
}

// runExec runs the command, recording its output.
func (e *Event) runExec(name string, args []string) error {
	// #nosec G204 -- the command is defined by the application's schedule,
	// not by request input.
	output, err := exec.Command(name, args...).CombinedOutput()

	e.mu.Lock()
	e.lastOutput = string(output)
	path := e.outputPath
	e.mu.Unlock()

	if path != "" {
		if writeErr := appendOutput(path, string(output)); writeErr != nil && err == nil {
			err = writeErr
		}
	}

	if err != nil {
		return fmt.Errorf("schedule: %s failed: %w: %s", name, err, strings.TrimSpace(string(output)))
	}
	return nil
}

// appendOutput appends a run's output to a log file.
func appendOutput(path, output string) error {
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("schedule: opening %s: %w", path, err)
	}
	defer func() { _ = file.Close() }()

	if !strings.HasSuffix(output, "\n") {
		output += "\n"
	}
	_, err = file.WriteString(output)
	return err
}

// AppendOutputTo appends each run's output to a file, so a scheduled
// command's output is not lost to the void cron sends it to.
func (e *Event) AppendOutputTo(path string) *Event {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.outputPath = path
	return e
}

// LastOutput returns the output of the most recent run.
func (e *Event) LastOutput() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.lastOutput
}

// Job schedules a queued job, so the work happens on a worker rather
// than in the scheduler process:
//
//	schedule.Job(queue, &jobs.GenerateReport{}).DailyAt("02:00")
func (s *Schedule) Job(q queue.Queue, job queue.Job) *Event {
	event := s.Call(func() error { return q.Push(job) })

	name := fmt.Sprintf("%T", job)
	if nameable, ok := job.(queue.Nameable); ok {
		name = nameable.JobName()
	}
	event.Description("job: " + name)

	return event
}

// --- hooks -----------------------------------------------------------

// Before runs a callback immediately before the event.
func (e *Event) Before(fn func()) *Event {
	e.beforeHooks = append(e.beforeHooks, fn)
	return e
}

// After runs a callback once the event finishes, whatever the outcome.
func (e *Event) After(fn func(err error)) *Event {
	e.afterHooks = append(e.afterHooks, fn)
	return e
}

// OnSuccess runs a callback when the event completes without error.
func (e *Event) OnSuccess(fn func()) *Event {
	e.successHooks = append(e.successHooks, fn)
	return e
}

// OnFailure runs a callback when the event returns an error.
func (e *Event) OnFailure(fn func(err error)) *Event {
	e.failureHooks = append(e.failureHooks, fn)
	return e
}

// --- one server ------------------------------------------------------

// UseCache gives the schedule a cache store to coordinate through, which
// is what OnOneServer needs. Use a store shared by every instance
// (Redis, or a database-backed store) - a per-process memory store
// coordinates nothing.
func (s *Schedule) UseCache(store cache.Store) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cache = store
	for _, event := range s.events {
		event.cache = store
	}
}

// OnOneServer runs the event on only one instance when several run the
// same schedule, by holding a lock in the shared cache store for the
// duration of the run.
//
// Without a cache store there is nothing to coordinate through, so the
// event runs normally rather than being silently skipped everywhere.
func (e *Event) OnOneServer() *Event {
	e.onOneServer = true
	return e
}

// oneServerLock returns the lock guarding this event, or nil when there
// is no store to coordinate through.
func (e *Event) oneServerLock() *cache.Lock {
	if !e.onOneServer || e.cache == nil {
		return nil
	}
	return cache.NewLock(e.cache, "schedule:"+e.label(), oneServerLockTTL)
}

// oneServerLockTTL is just under the scheduler's one-minute granularity:
// the lock is held for the rest of the minute the event fired in, so
// every other instance skips that firing, and it lapses in time for the
// next one. It is deliberately never released - releasing it at the end
// of the run would let a slower instance pick the same firing up.
const oneServerLockTTL = 55 * time.Second
