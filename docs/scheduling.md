# Task Scheduling

## Defining the schedule

```go
app.Register(&providers.ScheduleServiceProvider{
    Define: func(s *schedule.Schedule) {
        s.Call(pruneSessions).Daily().Description("prune expired sessions")
        s.Call(sendDigest).DailyAt("08:00")
        s.Call(syncOrders).EveryFiveMinutes()
        s.Call(weeklyReport).WeeklyOn(1, "09:00")   // Mondays 09:00
        s.Call(custom).Cron("*/15 8-18 * * 1-5")    // raw cron
    },
})
```

Frequencies: `EveryMinute`, `EveryFiveMinutes`, `EveryTenMinutes`,
`EveryThirtyMinutes`, `Hourly`, `HourlyAt(m)`, `Daily`, `DailyAt("15:04")`,
`Weekly`, `WeeklyOn(day, "15:04")`, `Monthly`, `Cron(expr)`.

## Running

Two options, mirroring Laravel:

```bash
# Option 1: keep a worker running
genesys schedule:work

# Option 2: system cron calls in every minute
* * * * * cd /srv/myapp && ./myapp schedule:run >> /dev/null 2>&1
```

`schedule:list` prints every registered task with its cron expression.

## Behaviour

- Overlapping runs of the same task are skipped by default; opt out with
  `.AllowOverlapping()`.
- Due tasks run concurrently; a panicking task is recovered and reported
  as an error.
- The cron parser supports `*`, `*/n`, ranges, lists, and steps across the
  standard 5 fields, with standard dom/dow OR semantics.

## Frequencies

Beyond the basics: `EveryFifteenMinutes`, `Weekdays`, `Weekends`,
`Mondays` … `Sundays`, `Days(1, 3, 5)`, `TwiceDaily(9, 17)`,
`Quarterly`, `Yearly`.

## Timezones

```go
schedule.Call(report).DailyAt("09:00").Timezone("Europe/Berlin")
```

Without it, a cron expression is evaluated in the process's local time.
An unknown zone name surfaces when the event runs, rather than the event
silently never firing.

## Conditions

```go
schedule.Call(sync).Hourly().When(func() bool { return featureEnabled() })
schedule.Call(sync).Hourly().Skip(func() bool { return maintenanceMode() })
schedule.Call(sync).EveryMinute().Between("09:00", "17:00")
schedule.Call(sync).EveryMinute().UnlessBetween("00:00", "06:00")
schedule.Call(sync).Daily().Environments("production")
```

A `Between` window whose end precedes its start wraps midnight, which is
how a night-time maintenance window is written.

## Commands and jobs

```go
schedule.Exec("pg_dump", "-Fc", "app").DailyAt("03:00").AppendOutputTo("storage/logs/backup.log")
schedule.Job(queue, &jobs.GenerateReport{}).Daily()
```

`Exec` keeps the command's combined output (`LastOutput()`) and treats a
non-zero exit as an error - a scheduled command's output otherwise goes
to the void cron sends it to. `Job` pushes onto the queue so the work
happens on a worker rather than in the scheduler process.

## Hooks

```go
schedule.Call(sync).Hourly().
    Before(func() { log.Info("starting sync") }).
    OnSuccess(func() { log.Info("sync done") }).
    OnFailure(func(err error) { log.Error("sync failed", "error", err) })
```

## Running on one instance

```go
schedule.Call(sendInvoices).Daily().OnOneServer()
```

The event takes a lock in the shared cache store for the rest of the
minute it fired in, so only one instance runs that firing. The lock is
deliberately not released at the end of the run: releasing it would let a
slower instance pick the same firing up.

This needs a store every instance shares (Redis, or a database-backed
store) - the `ScheduleServiceProvider` passes the configured cache store
along. With no store there is nothing to coordinate through, so the event
runs rather than being silently skipped everywhere.

## Testing a task

```
genesys schedule:test "nightly report"
```

Runs one task by description immediately, whether or not it is due.
