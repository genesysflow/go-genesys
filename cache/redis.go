package cache

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisConfig configures a Redis cache store.
type RedisConfig struct {
	// Addr is the host:port of the Redis server.
	Addr string
	// Password is the optional AUTH password.
	Password string
	// DB is the Redis database number.
	DB int
	// Prefix namespaces every key (default "cache:"), so Flush only
	// removes this store's keys.
	Prefix string
}

// RedisStore is a cache store backed by Redis. Values are stored as
// JSON, so decoded numbers come back as float64 like encoding/json.
type RedisStore struct {
	client *redis.Client
	prefix string
	ctx    context.Context
}

// NewRedisStore creates a Redis cache store from config.
func NewRedisStore(config RedisConfig) *RedisStore {
	client := redis.NewClient(&redis.Options{
		Addr:     config.Addr,
		Password: config.Password,
		DB:       config.DB,
	})
	return NewRedisStoreWithClient(client, config.Prefix)
}

// NewRedisStoreWithClient wraps an existing Redis client.
func NewRedisStoreWithClient(client *redis.Client, prefix string) *RedisStore {
	if prefix == "" {
		prefix = "cache:"
	}
	return &RedisStore{client: client, prefix: prefix, ctx: context.Background()}
}

// Client exposes the underlying Redis client.
func (s *RedisStore) Client() *redis.Client { return s.client }

func (s *RedisStore) key(key string) string { return s.prefix + key }

func decodeValue(raw string) any {
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return raw // stored by Increment or another client as a bare string
	}
	return value
}

// Get retrieves an item (nil when missing or expired).
func (s *RedisStore) Get(key string) (any, error) {
	raw, err := s.client.Get(s.ctx, s.key(key)).Result()
	if errors.Is(err, redis.Nil) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return decodeValue(raw), nil
}

// Put stores an item for the given TTL (<= 0 stores forever).
func (s *RedisStore) Put(key string, value any, ttl time.Duration) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if ttl <= 0 {
		ttl = 0 // no expiration
	}
	return s.client.Set(s.ctx, s.key(key), encoded, ttl).Err()
}

// Has reports whether a non-expired item exists.
func (s *RedisStore) Has(key string) (bool, error) {
	n, err := s.client.Exists(s.ctx, s.key(key)).Result()
	return n > 0, err
}

// Add stores an item only when absent; reports whether it stored.
func (s *RedisStore) Add(key string, value any, ttl time.Duration) (bool, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return false, err
	}
	if ttl <= 0 {
		ttl = 0
	}
	return s.client.SetNX(s.ctx, s.key(key), encoded, ttl).Result()
}

// Pull retrieves an item and removes it.
func (s *RedisStore) Pull(key string) (any, error) {
	raw, err := s.client.GetDel(s.ctx, s.key(key)).Result()
	if errors.Is(err, redis.Nil) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return decodeValue(raw), nil
}

// Forever stores an item without expiration.
func (s *RedisStore) Forever(key string, value any) error {
	return s.Put(key, value, 0)
}

// Increment atomically increases a numeric item, starting from zero.
func (s *RedisStore) Increment(key string, amount int64) (int64, error) {
	return s.client.IncrBy(s.ctx, s.key(key), amount).Result()
}

// hitScript counts a hit and gives the count its window when it has none:
// a new key, or one an INCR recreated without an expiry.
var hitScript = redis.NewScript(`
local n = redis.call('incr', KEYS[1])
if redis.call('pttl', KEYS[1]) < 0 then
	redis.call('pexpire', KEYS[1], ARGV[1])
end
return n
`)

// Hit counts a hit in a fixed window, atomically (WindowCounter).
func (s *RedisStore) Hit(key string, window time.Duration) (int64, error) {
	ms := window.Milliseconds()
	if ms < 1 {
		ms = 1
	}
	return hitScript.Run(s.ctx, s.client, []string{s.key(key)}, ms).Int64()
}

// Decrement atomically decreases a numeric item, starting from zero.
func (s *RedisStore) Decrement(key string, amount int64) (int64, error) {
	return s.client.DecrBy(s.ctx, s.key(key), amount).Result()
}

// Forget removes an item.
func (s *RedisStore) Forget(key string) error {
	return s.client.Del(s.ctx, s.key(key)).Err()
}

// forgetIfEqualsScript deletes the key only while it still holds the
// expected value - the atomic lock-release pattern from the Redis docs.
var forgetIfEqualsScript = redis.NewScript(`
if redis.call('get', KEYS[1]) == ARGV[1] then
	return redis.call('del', KEYS[1])
end
return 0
`)

// ForgetIfEquals atomically removes the key only while it still holds
// the given string value; the lock helper uses it so a Release can
// never free a lock that expired and was re-acquired by another owner.
func (s *RedisStore) ForgetIfEquals(key string, value string) (bool, error) {
	// Values are stored JSON-encoded (see Put/Add), so compare against
	// the encoded form.
	encoded, err := json.Marshal(value)
	if err != nil {
		return false, err
	}
	n, err := forgetIfEqualsScript.Run(s.ctx, s.client, []string{s.key(key)}, string(encoded)).Int()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// Flush removes every key under this store's prefix (not the whole DB).
func (s *RedisStore) Flush() error {
	iter := s.client.Scan(s.ctx, 0, s.prefix+"*", 100).Iterator()
	for iter.Next(s.ctx) {
		if err := s.client.Del(s.ctx, iter.Val()).Err(); err != nil {
			return err
		}
	}
	return iter.Err()
}

// Close closes the underlying client.
func (s *RedisStore) Close() error { return s.client.Close() }
