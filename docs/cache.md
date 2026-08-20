# Cache

## Configuration

```yaml
# config/cache.yaml
default: memory
stores:
  memory:
    driver: memory
  file:
    driver: file
    path: storage/cache
```

## Usage

```go
import "github.com/genesysflow/go-genesys/facades/cache"

cache.Put("key", value, 10*time.Minute)
cache.Forever("key", value)

value, err := cache.Get("key")      // nil when missing/expired
has, err := cache.Has("key")
value, err = cache.Pull("key")      // get + delete
stored, err := cache.Add("lock", 1, time.Minute) // only when absent

n, err := cache.Increment("hits")
n, err = cache.Decrement("stock", 3)

cache.Forget("key")
cache.Flush()
```

## Remember

Compute-once caching:

```go
users, err := cache.Remember("users:all", time.Minute, func() (any, error) {
    return loadUsersFromDB()
})

settings, err := cache.RememberForever("settings", loadSettings)
```

## Named stores and custom drivers

```go
disk := cache.Store("file")
disk.Put("report", data, time.Hour)

// Register a custom store implementation
manager.Register("redis", myRedisStore) // any cache.Store implementation
```

The file store persists entries as JSON, so values must round-trip through
`encoding/json` (numbers come back as `float64`).

## Redis Store

```yaml
# config/cache.yaml
default: redis
stores:
  redis:
    driver: redis
    addr: ${REDIS_ADDR:-localhost:6379}
    password: ${REDIS_PASSWORD:-}
    db: 0
    prefix: "cache:"
```

`Flush` only removes keys under the store's prefix, and
`Increment`/`Decrement` are atomic (INCRBY).

## Atomic Locks

```go
lock := cache.NewLock(store, "reports:generate", time.Minute)
if acquired, _ := lock.Get(); acquired {
    defer lock.Release()
    generateReports()
}

// Or serialise a critical section, waiting up to 5s:
cache.WithLock(store, "critical", time.Minute, 5*time.Second, func() error {
    return doExclusiveWork()
})
```

Locks acquire atomically (`Add`/SETNX), expire so a crashed holder can't
wedge them, and only the owner can release.

## Rate Limiting

`middleware.Throttle` persists counters in a cache store, so limits
survive restarts and are shared across instances on Redis:

```go
kernel.Use(middleware.Throttle(middleware.ThrottleConfig{
    Store:       store,
    Name:        "api",
    MaxRequests: 60,
    Window:      time.Minute,
}))
```

Responses carry `X-RateLimit-Limit`/`X-RateLimit-Remaining`; rejected
requests get 429 with `Retry-After`.
