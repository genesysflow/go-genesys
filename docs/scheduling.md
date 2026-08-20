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
