# Error Handling

## Returning HTTP errors

Handlers return errors; the framework error handler renders them:

```go
import "github.com/genesysflow/go-genesys/errors"

return errors.NotFound("User not found")            // 404
return errors.Unauthorized()                        // 401
return errors.Forbidden("This action is unauthorized.")
return errors.UnprocessableEntity("Cannot process")
return errors.BadRequest("Invalid cursor", err)     // wraps the cause
```

Wrapped errors keep their status: `fmt.Errorf("loading user: %w", errors.NotFound())`
still renders as 404.

## Content negotiation

- API clients (`Accept: application/json`, AJAX) receive JSON:
  `{"success": false, "error": "...", "status": 404}`
- Browsers (`Accept: text/html`) receive a styled HTML error page.

## Validation errors

`*errors.ValidationError` (as produced by `http.ValidateRequest`) always
renders as Laravel's 422 shape:

```json
{"message": "The given data was invalid.", "errors": {"email": ["..."]}}
```

## Debug mode

With `APP_DEBUG=true`, responses include the underlying error and a stack
trace (captured at the point of failure when the error was wrapped with
`errors.WithStack`). In production both are logged, never returned —
internal error text routinely leaks SQL, paths, and hostnames.

## Reporting

```go
handler.AddReporter(func(err error, ctx contracts.Context) {
    sentry.Capture(err)
})
handler.DontReport(context.Canceled)
```

Panics in handlers are recovered by the recovery middleware, logged with
the panicking stack, and rendered as 500s.
