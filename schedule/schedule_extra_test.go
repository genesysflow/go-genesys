package schedule_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/genesysflow/go-genesys/cache"
	"github.com/genesysflow/go-genesys/queue"
	"github.com/genesysflow/go-genesys/schedule"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// moment parses a wall-clock instant in UTC for the schedule tests.
func moment(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse("2006-01-02 15:04", value)
	require.NoError(t, err)
	return parsed
}

// --- frequencies -----------------------------------------------------

func TestExtraFrequencyHelpers(t *testing.T) {
	s := schedule.New()
	noop := func() error { return nil }

	cases := []struct {
		name   string
		event  *schedule.Event
		due    []string
		notDue []string
	}{
		{"weekdays", s.Call(noop).Weekdays(), []string{"2026-08-20 00:00"}, []string{"2026-08-22 00:00"}},
		{"weekends", s.Call(noop).Weekends(), []string{"2026-08-22 00:00"}, []string{"2026-08-20 00:00"}},
		{"twice daily", s.Call(noop).TwiceDaily(1, 13), []string{"2026-08-20 01:00", "2026-08-20 13:00"}, []string{"2026-08-20 12:00"}},
		{"quarterly", s.Call(noop).Quarterly(), []string{"2026-07-01 00:00"}, []string{"2026-08-01 00:00"}},
		{"yearly", s.Call(noop).Yearly(), []string{"2026-01-01 00:00"}, []string{"2026-02-01 00:00"}},
		{"every fifteen", s.Call(noop).EveryFifteenMinutes(), []string{"2026-08-20 00:15"}, []string{"2026-08-20 00:16"}},
		{"mondays", s.Call(noop).Mondays(), []string{"2026-08-17 00:00"}, []string{"2026-08-18 00:00"}},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			for _, when := range testCase.due {
				assert.True(t, testCase.event.IsDue(moment(t, when)), "should be due at %s", when)
			}
			for _, when := range testCase.notDue {
				assert.False(t, testCase.event.IsDue(moment(t, when)), "should not be due at %s", when)
			}
		})
	}
}

// --- timezone --------------------------------------------------------

// A task scheduled for 09:00 in a timezone must fire at 09:00 there, not
// at 09:00 UTC.
func TestTimezone(t *testing.T) {
	s := schedule.New()
	event := s.Call(func() error { return nil }).DailyAt("09:00").Timezone("Asia/Tokyo")

	tokyo, err := time.LoadLocation("Asia/Tokyo")
	require.NoError(t, err)

	// 00:00 UTC is 09:00 in Tokyo.
	assert.True(t, event.IsDue(time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC)))
	assert.False(t, event.IsDue(time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC)))
	assert.True(t, event.IsDue(time.Date(2026, 8, 20, 9, 0, 0, 0, tokyo)))
}

func TestInvalidTimezoneSurfacesOnRun(t *testing.T) {
	s := schedule.New()
	event := s.Call(func() error { return nil }).Timezone("Not/AZone")

	assert.False(t, event.IsDue(time.Now()))
	assert.Error(t, event.Run())
}

// --- constraints -----------------------------------------------------

func TestWhenAndSkip(t *testing.T) {
	s := schedule.New()

	allow := false
	when := s.Call(func() error { return nil }).EveryMinute().When(func() bool { return allow })
	assert.False(t, when.IsDue(time.Now()))
	allow = true
	assert.True(t, when.IsDue(time.Now()))

	skip := false
	unless := s.Call(func() error { return nil }).EveryMinute().Skip(func() bool { return skip })
	assert.True(t, unless.IsDue(time.Now()))
	skip = true
	assert.False(t, unless.IsDue(time.Now()))
}

func TestBetween(t *testing.T) {
	s := schedule.New()
	event := s.Call(func() error { return nil }).EveryMinute().Between("09:00", "17:00")

	assert.True(t, event.IsDue(moment(t, "2026-08-20 09:00")))
	assert.True(t, event.IsDue(moment(t, "2026-08-20 16:59")))
	assert.False(t, event.IsDue(moment(t, "2026-08-20 08:59")))
	assert.False(t, event.IsDue(moment(t, "2026-08-20 17:01")))
}

// A window that wraps midnight is a night-time maintenance window, not
// an empty one.
func TestBetweenWrappingMidnight(t *testing.T) {
	s := schedule.New()
	event := s.Call(func() error { return nil }).EveryMinute().Between("22:00", "04:00")

	assert.True(t, event.IsDue(moment(t, "2026-08-20 23:30")))
	assert.True(t, event.IsDue(moment(t, "2026-08-20 02:00")))
	assert.False(t, event.IsDue(moment(t, "2026-08-20 12:00")))
}

func TestUnlessBetween(t *testing.T) {
	s := schedule.New()
	event := s.Call(func() error { return nil }).EveryMinute().UnlessBetween("09:00", "17:00")

	assert.False(t, event.IsDue(moment(t, "2026-08-20 10:00")))
	assert.True(t, event.IsDue(moment(t, "2026-08-20 20:00")))
}

func TestEnvironments(t *testing.T) {
	s := schedule.New()
	s.SetEnvironment("production")

	production := s.Call(func() error { return nil }).EveryMinute().Environments("production")
	staging := s.Call(func() error { return nil }).EveryMinute().Environments("staging")

	assert.True(t, production.IsDue(time.Now()))
	assert.False(t, staging.IsDue(time.Now()))
}

// --- exec ------------------------------------------------------------

func TestExecRunsACommand(t *testing.T) {
	s := schedule.New()
	event := s.Exec("echo", "hello").EveryMinute()

	require.NoError(t, event.Run())
	assert.Contains(t, event.LastOutput(), "hello")
}

func TestExecFailureIsAnError(t *testing.T) {
	s := schedule.New()
	event := s.Exec("false").EveryMinute()

	assert.Error(t, event.Run())
}

func TestAppendOutputTo(t *testing.T) {
	path := filepath.Join(t.TempDir(), "schedule.log")

	s := schedule.New()
	event := s.Exec("echo", "written").EveryMinute().AppendOutputTo(path)

	require.NoError(t, event.Run())
	require.NoError(t, event.Run())

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, 2, strings.Count(string(content), "written"), "output should append, not truncate")
}

// --- job dispatch ----------------------------------------------------

type reportJob struct {
	Month string `json:"month"`
}

func (j *reportJob) JobName() string { return "report" }
func (j *reportJob) Handle() error   { return nil }

func TestJobDispatch(t *testing.T) {
	fake := queue.NewFake()

	s := schedule.New()
	event := s.Job(fake, &reportJob{Month: "august"}).Daily()

	require.NoError(t, event.Run())
	require.Len(t, fake.Pushed(), 1)
	assert.Equal(t, "report", fake.Pushed()[0].Name)
}

// --- hooks -----------------------------------------------------------

func TestSuccessAndFailureHooks(t *testing.T) {
	s := schedule.New()

	var trail []string
	ok := s.Call(func() error { return nil }).EveryMinute().
		Before(func() { trail = append(trail, "before") }).
		OnSuccess(func() { trail = append(trail, "success") }).
		OnFailure(func(err error) { trail = append(trail, "failure") })

	require.NoError(t, ok.Run())
	assert.Equal(t, []string{"before", "success"}, trail)

	trail = nil
	failing := s.Call(func() error { return assertErr }).EveryMinute().
		OnSuccess(func() { trail = append(trail, "success") }).
		OnFailure(func(err error) { trail = append(trail, "failure:"+err.Error()) })

	require.Error(t, failing.Run())
	assert.Equal(t, []string{"failure:boom"}, trail)
}

// --- one server ------------------------------------------------------

// Two instances running the same schedule must not both run a task
// marked OnOneServer.
func TestOnOneServer(t *testing.T) {
	store := cache.NewMemoryStore()

	runs := 0
	first := schedule.New()
	first.UseCache(store)
	firstEvent := first.Call(func() error { runs++; return nil }).EveryMinute().Description("nightly").OnOneServer()

	second := schedule.New()
	second.UseCache(store)
	secondEvent := second.Call(func() error { runs++; return nil }).EveryMinute().Description("nightly").OnOneServer()

	require.NoError(t, firstEvent.Run())
	require.NoError(t, secondEvent.Run())

	assert.Equal(t, 1, runs, "only one instance should run the task")
}

// Without a cache store there is nothing to coordinate through, so the
// task runs rather than being silently skipped everywhere.
func TestOnOneServerWithoutCacheStillRuns(t *testing.T) {
	s := schedule.New()

	runs := 0
	event := s.Call(func() error { runs++; return nil }).EveryMinute().OnOneServer()

	require.NoError(t, event.Run())
	assert.Equal(t, 1, runs)
}

var assertErr = errBoom{}

type errBoom struct{}

func (errBoom) Error() string { return "boom" }
