# Queues

## Defining jobs

A job is a struct whose exported fields are serialized into the payload:

```go
type SendEmailJob struct {
    Email   string `json:"email"`
    Message string `json:"message"`
}

func (j *SendEmailJob) Handle() error {
    return sendEmail(j.Email, j.Message)
}

// Optional overrides
func (j *SendEmailJob) Tries() int              { return 5 }
func (j *SendEmailJob) Backoff() time.Duration  { return 30 * time.Second }
func (j *SendEmailJob) JobName() string         { return "emails.send" }
```

Every job type must be registered before workers can deserialize it —
do it in an init or provider:

```go
queue.Register[SendEmailJob]()
```

## Dispatching

```go
import queuefacade "github.com/genesysflow/go-genesys/facades/queue"

queuefacade.Dispatch(&SendEmailJob{Email: "user@example.com"})
queuefacade.DispatchLater(10*time.Minute, &SendEmailJob{Email: "user@example.com"})
```

## Connections

```yaml
# config/queue.yaml
default: database
connections:
  sync:      {driver: sync}      # runs jobs inline (development/tests)
  memory:    {driver: memory}    # in-process, lost on restart
  database:
    driver: database
    table: jobs
    failed_table: failed_jobs
    queue: default
```

The database driver needs its tables — create them in a migration:

```go
func (m *CreateQueueTables) Up(builder *schema.Builder) error {
    return queue.CreateJobsTables(builder)
}
```

## Workers

```bash
genesys queue:work                       # process until Ctrl+C
genesys queue:work --tries=5 --sleep=2s
genesys queue:work --once                # single job
genesys queue:work --stop-when-empty     # drain then exit
```

Workers reserve jobs with a conditional update, so multiple workers can run
concurrently without double-processing. Panicking jobs are caught, retried
up to their tries, then moved to the failed table.

## Failed jobs

```bash
genesys queue:failed          # list
genesys queue:retry 42        # retry one
genesys queue:retry all       # retry everything
```

## Redis Driver

```yaml
# config/queue.yaml
default: redis
connections:
  redis:
    driver: redis
    addr: ${REDIS_ADDR:localhost:6379}
    prefix: "queues:"
```

Jobs live on a Redis list per queue; delayed and released jobs wait in a
sorted set until due. Failed jobs land on a Redis list with the same
`queue:failed` / `queue:retry` commands.

## Chains

`queue.Chain` runs jobs strictly one after another; a failing link stops
the rest:

```go
queue.Chain(q,
    &ProcessPodcast{ID: id},
    &OptimizePodcast{ID: id},
    &ReleasePodcast{ID: id},
)
```

## Batches

Batches track a group of jobs with completion callbacks:

```go
batch := queue.NewBatch().
    Then(func(b *queue.Batch) { log.Info("all imports done") }).
    Catch(func(b *queue.Batch, err error) { log.Error("import failed", "err", err) }).
    Finally(func(b *queue.Batch) { cleanup() })

batch.Dispatch(q, jobs...)

done, total := batch.Progress()
```

Progress lives in-process: callbacks fire when the batch's jobs are
worked in the same process (any driver); workers in other processes
still run the jobs.

## Maintenance Mode

```bash
genesys down --message "Upgrading" --retry 300 --secret letmein
genesys up
```

With `middleware.Maintenance` registered, every request gets a 503 (and
`Retry-After`) while the down file exists; `?secret=...` bypasses via a
cookie for operators.
