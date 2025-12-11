package contracts

import "time"

// Session defines the interface for session management.
type Session interface {
	// ID returns the session ID.
	ID() string

	// Regenerate regenerates the session ID.
	Regenerate() error

	// Get retrieves a value from the session.
	Get(key string) any

	// GetString retrieves a string value from the session.
	GetString(key string) string

	// GetInt retrieves an integer value from the session.
	GetInt(key string) int

	// GetBool retrieves a boolean value from the session.
	GetBool(key string) bool

	// Set stores a value in the session.
	Set(key string, value any) error

	// Has checks if a key exists in the session.
	Has(key string) bool

	// Pull retrieves and removes a value from the session.
	Pull(key string) any

	// Forget removes a value from the session.
	Forget(key string) error

	// Flush removes all values from the session.
	Flush() error

	// Flash stores a value for the next request only.
	Flash(key string, value any) error

	// Keep keeps specific flash data for another request.
	Keep(keys ...string) error

	// Reflash keeps all flash data for another request.
	Reflash() error

	// All returns all session data.
	All() map[string]any

	// Save saves the session.
	Save() error

	// Destroy destroys the session.
	Destroy() error

	// CreatedAt returns when the session was created.
	CreatedAt() time.Time

	// LastActivity returns the last activity time.
	LastActivity() time.Time
}

// SessionDriver defines the interface for session storage drivers.
type SessionDriver interface {
	// Read reads session data.
	Read(id string) (map[string]any, error)

	// Write writes session data.
	Write(id string, data map[string]any, lifetime time.Duration) error

	// Destroy destroys a session.
	Destroy(id string) error

	// GC garbage collects old sessions.
	GC(lifetime time.Duration) error
}

// SessionManager manages session creation and retrieval.
type SessionManager interface {
	// Start starts a new session or resumes an existing one.
	Start(id string) (Session, error)

	// Driver returns a specific session driver.
	Driver(name string) SessionDriver

	// Extend registers a custom session driver.
	Extend(name string, driver SessionDriver)
}
