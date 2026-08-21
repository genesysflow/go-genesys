package session

import (
	"crypto/sha256"
	"encoding/base64"
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

// FileStorage persists sessions as files on disk. It implements
// fiber.Storage and can be used as the session manager's storage driver.
type FileStorage struct {
	dir string
	mu  sync.Mutex
}

type fileRecord struct {
	Data      string `json:"data"` // base64-encoded payload
	ExpiresAt int64  `json:"expires_at"`
}

// NewFileStorage creates a file-backed session storage rooted at dir.
func NewFileStorage(dir string) (*FileStorage, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("session: cannot create directory %s: %w", dir, err)
	}
	return &FileStorage{dir: dir}, nil
}

func (s *FileStorage) path(key string) string {
	sum := sha256.Sum256([]byte(key))
	return filepath.Join(s.dir, hex.EncodeToString(sum[:])+".session")
}

// Get retrieves a session payload by key.
func (s *FileStorage) Get(key string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	raw, err := os.ReadFile(s.path(key))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var record fileRecord
	if err := json.Unmarshal(raw, &record); err != nil {
		os.Remove(s.path(key))
		return nil, nil
	}
	if record.ExpiresAt != 0 && time.Now().Unix() >= record.ExpiresAt {
		os.Remove(s.path(key))
		return nil, nil
	}
	return base64.StdEncoding.DecodeString(record.Data)
}

// Set stores a session payload with an expiration (0 = no expiry).
func (s *FileStorage) Set(key string, val []byte, exp time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	record := fileRecord{Data: base64.StdEncoding.EncodeToString(val)}
	if exp > 0 {
		record.ExpiresAt = time.Now().Add(exp).Unix()
	}
	raw, err := json.Marshal(record)
	if err != nil {
		return err
	}
	tmp := s.path(key) + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path(key))
}

// Delete removes a session.
func (s *FileStorage) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	err := os.Remove(s.path(key))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

// Reset removes all sessions.
func (s *FileStorage) Reset() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".session" {
			continue
		}
		if err := os.Remove(filepath.Join(s.dir, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

// Close is a no-op for file storage.
func (s *FileStorage) Close() error {
	return nil
}
