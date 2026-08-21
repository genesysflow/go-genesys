package session

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisStorage stores sessions in Redis with native expiration. It
// implements fiber.Storage and can be used as the session manager's
// storage driver.
type RedisStorage struct {
	client *redis.Client
	prefix string
	ctx    context.Context
}

// RedisStorageConfig configures a Redis session storage.
type RedisStorageConfig struct {
	// Addr is the host:port of the Redis server.
	Addr string
	// Password is the optional AUTH password.
	Password string
	// DB is the Redis database number.
	DB int
	// Prefix namespaces every session key (default "session:").
	Prefix string
}

// NewRedisStorage creates a Redis session storage from config.
func NewRedisStorage(config RedisStorageConfig) *RedisStorage {
	client := redis.NewClient(&redis.Options{
		Addr:     config.Addr,
		Password: config.Password,
		DB:       config.DB,
	})
	return NewRedisStorageWithClient(client, config.Prefix)
}

// NewRedisStorageWithClient wraps an existing Redis client.
func NewRedisStorageWithClient(client *redis.Client, prefix string) *RedisStorage {
	if prefix == "" {
		prefix = "session:"
	}
	return &RedisStorage{client: client, prefix: prefix, ctx: context.Background()}
}

// Get retrieves a session by key (nil when missing or expired).
func (s *RedisStorage) Get(key string) ([]byte, error) {
	raw, err := s.client.Get(s.ctx, s.prefix+key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, nil
	}
	return raw, err
}

// Set stores a session with the given expiration.
func (s *RedisStorage) Set(key string, value []byte, expiration time.Duration) error {
	if expiration < 0 {
		expiration = 0
	}
	return s.client.Set(s.ctx, s.prefix+key, value, expiration).Err()
}

// Delete removes a session.
func (s *RedisStorage) Delete(key string) error {
	return s.client.Del(s.ctx, s.prefix+key).Err()
}

// Reset removes all sessions under this storage's prefix.
func (s *RedisStorage) Reset() error {
	iter := s.client.Scan(s.ctx, 0, s.prefix+"*", 100).Iterator()
	for iter.Next(s.ctx) {
		if err := s.client.Del(s.ctx, iter.Val()).Err(); err != nil {
			return err
		}
	}
	return iter.Err()
}

// Close closes the underlying client.
func (s *RedisStorage) Close() error {
	return s.client.Close()
}
