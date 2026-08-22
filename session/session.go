// Package session provides session management using Fiber's session middleware.
package session

import (
	"encoding/gob"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/session"
)

func init() {
	// Flash data and old input are stored as maps; register them so
	// serializing storages (file, database) can encode session payloads.
	gob.Register(map[string]any{})
	gob.Register([]any{})
}

// Session wraps Fiber's session with Laravel-like API.
type Session struct {
	store     *session.Session
	sess      *session.Store
	id        string
	data      map[string]any
	oldFlash  map[string]any // flashed on the previous request; visible now, gone after Save
	newFlash  map[string]any // flashed on this request; visible now and next request
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

	// Storage is the storage driver name: memory, file, or database.
	Storage string

	// Path is the directory for the file storage driver
	// (default "storage/sessions").
	Path string

	// Table is the table name for the database storage driver
	// (default "sessions").
	Table string

	// CustomStorage overrides the driver selection with an explicit
	// fiber.Storage implementation (used for the database driver, which
	// needs a live connection).
	CustomStorage fiber.Storage
}

// DefaultConfig returns the default session configuration.
//
// CookieSecure defaults to true. A session cookie sent over plaintext HTTP is
// readable by anyone on the network path and replayable as a full account
// takeover, so the secure attribute is the safe default and insecure transport
// is the thing that must be opted into. Local development over http:// needs
// `session.secure: false` in config; production should never set it.
func DefaultConfig() Config {
	return Config{
		Expiration:     24 * time.Hour,
		CookieName:     "genesys_session",
		CookiePath:     "/",
		CookieSecure:   true,
		CookieHTTPOnly: true,
		CookieSameSite: "Lax",
		KeyLookup:      "cookie:genesys_session",
		Storage:        "memory",
	}
}

// Config returns the manager's configuration, so a caller can see what
// it resolved to - which store, which cookie, which directory.
func (m *Manager) Config() Config {
	return m.config
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

	fiberConfig := session.Config{
		Expiration:     cfg.Expiration,
		CookiePath:     cfg.CookiePath,
		CookieDomain:   cfg.CookieDomain,
		CookieSecure:   cfg.CookieSecure,
		CookieHTTPOnly: cfg.CookieHTTPOnly,
		CookieSameSite: cfg.CookieSameSite,
		KeyLookup:      cfg.KeyLookup,
	}

	if cfg.CustomStorage != nil {
		fiberConfig.Storage = cfg.CustomStorage
	} else if cfg.Storage == "file" {
		path := cfg.Path
		if path == "" {
			path = "storage/sessions"
		}
		if storage, err := NewFileStorage(path); err == nil {
			fiberConfig.Storage = storage
		}
	}
	// "memory" (and unknown drivers) use Fiber's in-memory default.

	store := session.New(fiberConfig)

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
		oldFlash:  make(map[string]any),
		newFlash:  make(map[string]any),
		createdAt: time.Now(),
	}

	// Data flashed on the previous request is visible for this request only.
	if flashData := sess.Get("_flash"); flashData != nil {
		if fm, ok := flashData.(map[string]any); ok {
			s.oldFlash = fm
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
		handlerErr := c.Next()

		// The session is saved whatever the handler returned. A handler
		// that fails still needs what it wrote to survive: a validation
		// failure flashes its errors and old input on the way out, and
		// dropping them here would land the user on a form with an empty
		// error bag.
		if saveErr := sess.Save(); saveErr != nil && handlerErr == nil {
			return saveErr
		}

		return handlerErr
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

// Get retrieves a value from the session. Flash data (from this request or
// the previous one) takes precedence over persistent data.
func (s *Session) Get(key string) any {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if val, ok := s.newFlash[key]; ok {
		return val
	}
	if val, ok := s.oldFlash[key]; ok {
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
	s.oldFlash = make(map[string]any)
	s.newFlash = make(map[string]any)
	s.dirty = true
	return nil
}

// Flash stores a value visible for the rest of this request and the next
// request only.
func (s *Session) Flash(key string, value any) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.newFlash[key] = value
	s.dirty = true
	return nil
}

// Keep extends specific flash data from the previous request for one more
// request.
func (s *Session) Keep(keys ...string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, key := range keys {
		if val, ok := s.oldFlash[key]; ok {
			s.newFlash[key] = val
		}
	}
	s.dirty = true
	return nil
}

// Reflash extends all flash data from the previous request for one more
// request.
func (s *Session) Reflash() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for key, val := range s.oldFlash {
		if _, exists := s.newFlash[key]; !exists {
			s.newFlash[key] = val
		}
	}
	s.dirty = true
	return nil
}

// FlashInput flashes request input for the next request (repopulating forms
// after a validation redirect).
func (s *Session) FlashInput(input map[string]any) error {
	return s.Flash("_old_input", input)
}

// Old returns a previously flashed input value, or the default when absent.
func (s *Session) Old(key string, defaultValue ...string) string {
	if input, ok := s.Get("_old_input").(map[string]any); ok {
		if value, ok := input[key]; ok {
			if str, ok := value.(string); ok {
				return str
			}
		}
	}
	if len(defaultValue) > 0 {
		return defaultValue[0]
	}
	return ""
}

// HasOld reports whether input was flashed for a specific key.
//
// Answered by key rather than by comparing the value against a sentinel,
// so a flashed empty string is still input and no real value can be
// mistaken for absence.
func (s *Session) HasOld(key string) bool {
	input, ok := s.Get("_old_input").(map[string]any)
	if !ok {
		return false
	}
	_, present := input[key]
	return present
}

// HasOldInput reports whether any input was flashed on the previous request.
func (s *Session) HasOldInput() bool {
	_, ok := s.Get("_old_input").(map[string]any)
	return ok
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

	// Only data flashed this request survives into the next one; the
	// previous request's flash data ages out here.
	if len(s.newFlash) > 0 {
		s.store.Set("_flash", s.newFlash)
	} else {
		s.store.Delete("_flash")
	}
	s.oldFlash = make(map[string]any)
	s.newFlash = make(map[string]any)

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
