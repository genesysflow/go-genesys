package session_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/genesysflow/go-genesys/session"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func storages(t *testing.T) map[string]fiber.Storage {
	t.Helper()

	fileStorage, err := session.NewFileStorage(t.TempDir())
	require.NoError(t, err)

	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	_, err = db.Exec(`CREATE TABLE sessions (id TEXT PRIMARY KEY, payload TEXT NOT NULL, expires_at INTEGER NOT NULL)`)
	require.NoError(t, err)

	return map[string]fiber.Storage{
		"file":     fileStorage,
		"database": session.NewDatabaseStorage("sqlite", &sqlExecutor{db}, "sessions"),
	}
}

type sqlExecutor struct{ db *sql.DB }

func (e *sqlExecutor) Query(q string, b ...any) (*sql.Rows, error) { return e.db.Query(q, b...) }
func (e *sqlExecutor) QueryRow(q string, b ...any) *sql.Row        { return e.db.QueryRow(q, b...) }
func (e *sqlExecutor) Exec(q string, b ...any) (sql.Result, error) { return e.db.Exec(q, b...) }

func TestStorageBehaviour(t *testing.T) {
	for name, storage := range storages(t) {
		t.Run(name, func(t *testing.T) {
			// Missing keys return nil without error.
			value, err := storage.Get("missing")
			require.NoError(t, err)
			assert.Nil(t, value)

			// Round trip binary payloads.
			payload := []byte{0x00, 0x01, 0xFF, 0xFE, 'g', 'o'}
			require.NoError(t, storage.Set("sid", payload, time.Minute))
			value, err = storage.Get("sid")
			require.NoError(t, err)
			assert.Equal(t, payload, value)

			// Overwrite existing keys.
			require.NoError(t, storage.Set("sid", []byte("second"), time.Minute))
			value, err = storage.Get("sid")
			require.NoError(t, err)
			assert.Equal(t, []byte("second"), value)

			// Expired entries behave as missing. Expiry has one-second
			// granularity: a 1ns TTL truncates to "now", which Get treats
			// as already expired.
			require.NoError(t, storage.Set("gone", []byte("x"), time.Nanosecond))
			value, err = storage.Get("gone")
			require.NoError(t, err)
			assert.Nil(t, value)

			// Delete and Reset.
			require.NoError(t, storage.Delete("sid"))
			value, err = storage.Get("sid")
			require.NoError(t, err)
			assert.Nil(t, value)

			require.NoError(t, storage.Set("a", []byte("1"), time.Minute))
			require.NoError(t, storage.Set("b", []byte("2"), time.Minute))
			require.NoError(t, storage.Reset())
			value, err = storage.Get("a")
			require.NoError(t, err)
			assert.Nil(t, value)
		})
	}
}

func TestLazyStorageResolvesOnce(t *testing.T) {
	builds := 0
	lazy := session.NewLazyStorage(func() (fiber.Storage, error) {
		builds++
		return session.NewFileStorage(t.TempDir())
	})

	require.NoError(t, lazy.Set("k", []byte("v"), time.Minute))
	value, err := lazy.Get("k")
	require.NoError(t, err)
	assert.Equal(t, []byte("v"), value)
	assert.Equal(t, 1, builds)
}
