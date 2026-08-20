# HTTP Client

A fluent client for outbound requests, mirroring Laravel's `Http` facade.

```go
import "github.com/genesysflow/go-genesys/client"

resp, err := client.New().
    BaseURL("https://api.example.com").
    WithToken(apiToken).
    WithHeader("X-Request-Id", requestID).
    WithQuery("page", "2").
    Timeout(10 * time.Second).
    Retry(3, time.Second).
    Get("/users")

if err != nil {
    return err // network failure after all retries
}
if resp.Failed() {
    return fmt.Errorf("api returned %d: %s", resp.Status(), resp.String())
}

var users []User
if err := resp.JSON(&users); err != nil {
    return err
}
```

## Sending bodies

```go
// JSON (default for maps/structs)
resp, err := client.New().Post(url, map[string]string{"name": "Ada"})

// Form-encoded
resp, err = client.New().AsForm().Post(url, map[string]string{"name": "Ada"})

// Raw
resp, err = client.New().WithHeader("Content-Type", "text/plain").Post(url, "raw body")
```

## Retries

`Retry(times, delay)` retries network errors and 5xx responses; 4xx
responses are returned immediately (client errors will not succeed on
retry).

## Response helpers

```go
resp.Status()        // int
resp.Body()          // []byte
resp.String()        // string
resp.JSON(&target)   // unmarshal
resp.Header("ETag")
resp.Successful()    // 2xx
resp.ClientError()   // 4xx
resp.ServerError()   // 5xx
```

Shorthands `client.Get(url)` and `client.Post(url, body)` use default
settings.
