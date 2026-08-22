# Dev Tools

A local panel showing what the application just did: the requests it
handled and the queries they ran.

It is a development tool. The panel serves request paths and SQL, which
is exactly what must not leave a production box, so `Register` refuses to
mount it there.

## Setup

```go
recorder := devtools.NewRecorder(100)   // ring buffer, oldest dropped

kernel.Use(devtools.Middleware(recorder))
manager.Listen(recorder.RecordQuery)    // the DB::listen hook

if err := devtools.Register(kernel, recorder, "/_genesys"); err != nil {
    // production: the panel is not mounted
    log.Info(err.Error())
}
```

Applications usually register routes on a router rather than the kernel;
`RegisterRoutes` mounts the panel there and refuses in production for the
same reason:

```go
func Devtools(r *http.Router) {
    r.Use(devtools.Middleware(recorder))
    _ = devtools.RegisterRoutes(r, recorder, "/_genesys")
}
```

Then open `/_genesys`.

## What it records

| | |
|---|---|
| Requests | method, path, status, duration |
| Queries | connection, SQL, binding count, duration, error |

Query bindings are withheld unless you ask for them:

```go
recorder.ShowBindings = true
```

They routinely carry personal data, and the SQL text is usually what is
being debugged.

## Correlating queries to requests

```go
queries := recorder.QueriesDuring(entry)
```

The database listener carries no request identity, so this is a time
window rather than a true correlation: under concurrent traffic it can
include another request's queries. In local development, where the panel
is used, that is rarely a distinction that matters.
