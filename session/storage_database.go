package session

import (
	"encoding/base64"
	"fmt"
	"time"

	"github.com/genesysflow/go-genesys/database/schema"
	"github.com/genesysflow/go-genesys/query"
	"github.com/gofiber/fiber/v2"
)

// DatabaseStorage persists sessions in a database table. It implements
// fiber.Storage.
//
// Expected schema (see session.CreateSessionsTable):
//
//	sessions: id (varchar primary), payload (text), expires_at (bigint)
type DatabaseStorage struct {
	driver   string
	executor query.Executor
	table    string
}

// NewDatabaseStorage creates a database-backed session storage.
func NewDatabaseStorage(driver string, executor query.Executor, table ...string) *DatabaseStorage {
	name := "sessions"
	if len(table) > 0 && table[0] != "" {
		name = table[0]
	}
	return &DatabaseStorage{driver: driver, executor: executor, table: name}
}

func (s *DatabaseStorage) rows() *query.Builder {
	return query.New(s.driver, s.executor).Table(s.table)
}

// Get retrieves a session payload by key.
func (s *DatabaseStorage) Get(key string) ([]byte, error) {
	row, err := s.rows().Where("id", key).First()
	if err != nil {
		if err == query.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	if expires := toUnix(row["expires_at"]); expires != 0 && time.Now().Unix() >= expires {
		s.Delete(key)
		return nil, nil
	}
	return base64.StdEncoding.DecodeString(fmt.Sprint(row["payload"]))
}

// Set stores a session payload with an expiration (0 = no expiry).
func (s *DatabaseStorage) Set(key string, val []byte, exp time.Duration) error {
	payload := base64.StdEncoding.EncodeToString(val)
	var expiresAt int64
	if exp > 0 {
		expiresAt = time.Now().Add(exp).Unix()
	}

	affected, err := s.rows().Where("id", key).Update(map[string]any{
		"payload":    payload,
		"expires_at": expiresAt,
	})
	if err != nil {
		return err
	}
	if affected > 0 {
		return nil
	}
	return s.rows().Insert(map[string]any{
		"id":         key,
		"payload":    payload,
		"expires_at": expiresAt,
	})
}

// Delete removes a session.
func (s *DatabaseStorage) Delete(key string) error {
	_, err := s.rows().Where("id", key).Delete()
	return err
}

// Reset removes all sessions.
func (s *DatabaseStorage) Reset() error {
	_, err := s.rows().Delete()
	return err
}

// Close is a no-op for database storage.
func (s *DatabaseStorage) Close() error {
	return nil
}

// GC removes expired sessions; call it periodically (e.g. from the scheduler).
func (s *DatabaseStorage) GC() error {
	_, err := s.rows().
		Where("expires_at", "!=", 0).
		Where("expires_at", "<=", time.Now().Unix()).
		Delete()
	return err
}

// CreateSessionsTable creates the sessions table used by the database
// session driver. Call it from an application migration.
func CreateSessionsTable(builder *schema.Builder, table ...string) error {
	name := "sessions"
	if len(table) > 0 && table[0] != "" {
		name = table[0]
	}
	return builder.Create(name, func(t *schema.Blueprint) {
		t.String("id", 255)
		t.Text("payload")
		t.BigInteger("expires_at")
		t.Primary("id")
	})
}

// LazyStorage defers building the real storage until first use, so storage
// that depends on services registered later in bootstrap (e.g. the database)
// can be configured before those services exist.
type LazyStorage struct {
	build   func() (fiber.Storage, error)
	storage fiber.Storage
}

// NewLazyStorage creates a storage that is built on first use.
func NewLazyStorage(build func() (fiber.Storage, error)) *LazyStorage {
	return &LazyStorage{build: build}
}

func (s *LazyStorage) resolve() (fiber.Storage, error) {
	if s.storage != nil {
		return s.storage, nil
	}
	storage, err := s.build()
	if err != nil {
		return nil, err
	}
	s.storage = storage
	return storage, nil
}

// Get retrieves a session payload by key.
func (s *LazyStorage) Get(key string) ([]byte, error) {
	storage, err := s.resolve()
	if err != nil {
		return nil, err
	}
	return storage.Get(key)
}

// Set stores a session payload.
func (s *LazyStorage) Set(key string, val []byte, exp time.Duration) error {
	storage, err := s.resolve()
	if err != nil {
		return err
	}
	return storage.Set(key, val, exp)
}

// Delete removes a session.
func (s *LazyStorage) Delete(key string) error {
	storage, err := s.resolve()
	if err != nil {
		return err
	}
	return storage.Delete(key)
}

// Reset removes all sessions.
func (s *LazyStorage) Reset() error {
	storage, err := s.resolve()
	if err != nil {
		return err
	}
	return storage.Reset()
}

// Close closes the underlying storage when it was built.
func (s *LazyStorage) Close() error {
	if s.storage == nil {
		return nil
	}
	return s.storage.Close()
}

func toUnix(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case int:
		return int64(n)
	case float64:
		return int64(n)
	case string:
		var out int64
		fmt.Sscanf(n, "%d", &out)
		return out
	case []byte:
		var out int64
		fmt.Sscanf(string(n), "%d", &out)
		return out
	}
	return 0
}
