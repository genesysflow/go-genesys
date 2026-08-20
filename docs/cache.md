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
