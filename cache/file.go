package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// FileStore persists cache entries as JSON files on disk.
// Values must round-trip through encoding/json; numbers come back as
// float64 (or json.Number semantics for Increment/Decrement).
type FileStore struct {
	dir string
	mu  sync.Mutex
}

type fileEntry struct {
	Value     any   `json:"value"`
	ExpiresAt int64 `json:"expires_at"` // unix seconds, 0 = forever
}

// NewFileStore creates a file cache store rooted at dir.
func NewFileStore(dir string) (*FileStore, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("cache: cannot create directory %s: %w", dir, err)
	}
	return &FileStore{dir: dir}, nil
}

func (s *FileStore) path(key string) string {
	sum := sha256.Sum256([]byte(key))
	return filepath.Join(s.dir, hex.EncodeToString(sum[:])+".cache")
}

func (s *FileStore) read(key string) (*fileEntry, error) {
	data, err := os.ReadFile(s.path(key))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var entry fileEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		// A corrupt entry behaves like a miss.
		os.Remove(s.path(key))
		return nil, nil
	}
	if entry.ExpiresAt != 0 && time.Now().Unix() >= entry.ExpiresAt {
		os.Remove(s.path(key))
		return nil, nil
	}
	return &entry, nil
}

func (s *FileStore) write(key string, entry fileEntry) error {
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("cache: value for %q is not JSON-serializable: %w", key, err)
	}
	tmp := s.path(key) + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path(key))
}

// Get retrieves an item from the cache.
func (s *FileStore) Get(key string) (any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, err := s.read(key)
	if err != nil || entry == nil {
		return nil, err
	}
	return entry.Value, nil
}

// Put stores an item for the given TTL (0 or negative = forever).
func (s *FileStore) Put(key string, value any, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.write(key, newFileEntry(value, ttl))
}

// Has reports whether a non-expired item exists.
func (s *FileStore) Has(key string) (bool, error) {
	value, err := s.Get(key)
	return value != nil, err
}

// Add stores the item only when absent; returns true when stored.
func (s *FileStore) Add(key string, value any, ttl time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, err := s.read(key)
	if err != nil {
		return false, err
	}
	if entry != nil {
		return false, nil
	}
	return true, s.write(key, newFileEntry(value, ttl))
}

// Pull retrieves an item and removes it.
func (s *FileStore) Pull(key string) (any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, err := s.read(key)
	if err != nil || entry == nil {
		return nil, err
	}
	if err := os.Remove(s.path(key)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	return entry.Value, nil
}

// Forever stores an item without expiration.
func (s *FileStore) Forever(key string, value any) error {
	return s.Put(key, value, 0)
}

// Increment increases a numeric item, starting from zero when missing.
func (s *FileStore) Increment(key string, amount int64) (int64, error) {
	return s.incrementBy(key, amount)
}

// Decrement decreases a numeric item, starting from zero when missing.
func (s *FileStore) Decrement(key string, amount int64) (int64, error) {
	return s.incrementBy(key, -amount)
}

func (s *FileStore) incrementBy(key string, amount int64) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var current int64
	var expiresAt int64
	entry, err := s.read(key)
	if err != nil {
		return 0, err
	}
	if entry != nil {
		n, err := toNumber(entry.Value)
		if err != nil {
			return 0, err
		}
		current = n
		expiresAt = entry.ExpiresAt
	}
	current += amount
	if err := s.write(key, fileEntry{Value: current, ExpiresAt: expiresAt}); err != nil {
		return 0, err
	}
	return current, nil
}

// Forget removes an item from the cache.
func (s *FileStore) Forget(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	err := os.Remove(s.path(key))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

// Flush removes all cache files in the store's directory.
func (s *FileStore) Flush() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".cache" {
			continue
		}
		if err := os.Remove(filepath.Join(s.dir, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func newFileEntry(value any, ttl time.Duration) fileEntry {
	entry := fileEntry{Value: value}
	if ttl > 0 {
		entry.ExpiresAt = time.Now().Add(ttl).Unix()
	}
	return entry
}
