// Package session provides session management using Fiber's session middleware.
package session

import (
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/session"
)

// Session wraps Fiber's session with Laravel-like API.
type Session struct {
	store     *session.Session
	sess      *session.Store
	id        string
	data      map[string]any
	flash     map[string]any
	mu        sync.RWMutex
	createdAt time.Time
	dirty     bool
}

// Manager manages sessions.
type Manager struct {
	store   *session.Store
	config  Config
	drivers map[string]Driver
}

// Config holds session configuration.
type Config struct {
	// Expiration is the session expiration time.
	Expiration time.Duration

	// CookieName is the name of the session cookie.
	CookieName string

	// CookiePath is the path of the session cookie.
	CookiePath string

	// CookieDomain is the domain of the session cookie.
	CookieDomain string

	// CookieSecure indicates if the cookie should only be sent over HTTPS.
	CookieSecure bool

	// CookieHTTPOnly indicates if the cookie should be HTTP only.
	CookieHTTPOnly bool

	// CookieSameSite controls the SameSite attribute.
	CookieSameSite string

	// KeyLookup is the key lookup format (e.g., "cookie:session_id").
	KeyLookup string

	// Storage is the storage driver name.
	Storage string
}

// DefaultConfig returns the default session configuration.
func DefaultConfig() Config {
	return Config{
		Expiration:     24 * time.Hour,
		CookieName:     "genesys_session",
		CookiePath:     "/",
		CookieSecure:   false,
		CookieHTTPOnly: true,
		CookieSameSite: "Lax",
		KeyLookup:      "cookie:genesys_session",
		Storage:        "memory",
	}
}

// NewManager creates a new session manager.
func NewManager(config ...Config) *Manager {
	cfg := DefaultConfig()
	if len(config) > 0 {
		cfg = config[0]
	}

	if cfg.CookieName != "" {
		cfg.KeyLookup = "cookie:" + cfg.CookieName
	}

	store := session.New(session.Config{
		Expiration:     cfg.Expiration,
		CookiePath:     cfg.CookiePath,
		CookieDomain:   cfg.CookieDomain,
		CookieSecure:   cfg.CookieSecure,
		CookieHTTPOnly: cfg.CookieHTTPOnly,
		CookieSameSite: cfg.CookieSameSite,
		KeyLookup:      cfg.KeyLookup,
	})

	return &Manager{
		store:   store,
		config:  cfg,
		drivers: make(map[string]Driver),
	}
}

// Store returns the underlying Fiber session store.
func (m *Manager) Store() *session.Store {
	return m.store
}

// Get retrieves or creates a session for the given Fiber context.
func (m *Manager) Get(c *fiber.Ctx) (*Session, error) {
	sess, err := m.store.Get(c)
	if err != nil {
		return nil, err
	}

	s := &Session{
		store:     sess,
		sess:      m.store,
		id:        sess.ID(),
		data:      make(map[string]any),
		flash:     make(map[string]any),
		createdAt: time.Now(),
	}

	// Load existing flash data
	if flashData := sess.Get("_flash"); flashData != nil {
		if fm, ok := flashData.(map[string]any); ok {
			s.flash = fm
		}
	}

	return s, nil
}

// Middleware returns Fiber middleware for session handling.
func (m *Manager) Middleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		sess, err := m.Get(c)
		if err != nil {
			return err
		}

		// Store session in locals
		c.Locals("session", sess)

		// Continue to next handler
		if err := c.Next(); err != nil {
			return err
		}

		// Save session
		return sess.Save()
	}
}

// ID returns the session ID.
func (s *Session) ID() string {
	return s.id
}

// Regenerate regenerates the session ID.
func (s *Session) Regenerate() error {
	return s.store.Regenerate()
}

// Get retrieves a value from the session.
func (s *Session) Get(key string) any {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Check flash first
	if val, ok := s.flash[key]; ok {
		return val
	}

	return s.store.Get(key)
}

// GetString retrieves a string value from the session.
func (s *Session) GetString(key string) string {
	val := s.Get(key)
	if val == nil {
		return ""
	}
	if str, ok := val.(string); ok {
		return str
	}
	return ""
}

// GetInt retrieves an integer value from the session.
func (s *Session) GetInt(key string) int {
	val := s.Get(key)
	if val == nil {
		return 0
	}
	if i, ok := val.(int); ok {
		return i
	}
	return 0
}

// GetBool retrieves a boolean value from the session.
func (s *Session) GetBool(key string) bool {
	val := s.Get(key)
	if val == nil {
		return false
	}
	if b, ok := val.(bool); ok {
		return b
	}
	return false
}

// Set stores a value in the session.
func (s *Session) Set(key string, value any) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.store.Set(key, value)
	s.dirty = true
	return nil
}

// Has checks if a key exists in the session.
func (s *Session) Has(key string) bool {
	return s.Get(key) != nil
}

// Pull retrieves and removes a value from the session.
func (s *Session) Pull(key string) any {
	s.mu.Lock()
	defer s.mu.Unlock()

	val := s.store.Get(key)
	s.store.Delete(key)
	s.dirty = true
	return val
}

// Forget removes a value from the session.
func (s *Session) Forget(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.store.Delete(key)
	s.dirty = true
	return nil
}

// Flush removes all values from the session.
func (s *Session) Flush() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	keys := s.store.Keys()
	for _, key := range keys {
		s.store.Delete(key)
	}
	s.flash = make(map[string]any)
	s.dirty = true
	return nil
}

// Flash stores a value for the next request only.
func (s *Session) Flash(key string, value any) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.flash[key] = value
	s.dirty = true
	return nil
}

// Keep keeps specific flash data for another request.
func (s *Session) Keep(keys ...string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, key := range keys {
		if val, ok := s.flash[key]; ok {
			s.flash[key] = val
		}
	}
	s.dirty = true
	return nil
}

// Reflash keeps all flash data for another request.
func (s *Session) Reflash() error {
	// Flash data is already in s.flash, nothing to do
	s.dirty = true
	return nil
}

// All returns all session data.
func (s *Session) All() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data := make(map[string]any)
	for _, key := range s.store.Keys() {
		data[key] = s.store.Get(key)
	}
	return data
}

// Save saves the session.
func (s *Session) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Save flash data for next request
	if len(s.flash) > 0 {
		s.store.Set("_flash", s.flash)
	} else {
		s.store.Delete("_flash")
	}

	// Clear flash after saving (it was for this request)
	s.flash = make(map[string]any)

	return s.store.Save()
}

// Destroy destroys the session.
func (s *Session) Destroy() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.store.Destroy()
}

// CreatedAt returns when the session was created.
func (s *Session) CreatedAt() time.Time {
	return s.createdAt
}

// LastActivity returns the last activity time.
func (s *Session) LastActivity() time.Time {
	// For now, return current time
	// A full implementation would track this
	return time.Now()
}

// Driver is the interface for session storage drivers.
type Driver interface {
	// Get retrieves session data.
	Get(id string) (map[string]any, error)

	// Set stores session data.
	Set(id string, data map[string]any, expiration time.Duration) error

	// Delete removes session data.
	Delete(id string) error

	// Clear removes all sessions.
	Clear() error
}

// GetFromContext retrieves the session from Fiber context.
func GetFromContext(c *fiber.Ctx) *Session {
	sess := c.Locals("session")
	if sess == nil {
		return nil
	}
	return sess.(*Session)
}
