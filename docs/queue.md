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
