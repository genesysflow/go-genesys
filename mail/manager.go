package mail

import (
	"errors"
	"fmt"
	"sync"
)

// Manager holds named mailers with a default - Laravel's
// Mail::mailer("postmark"):
//
//	manager := mail.NewManager()
//	manager.Register("smtp", smtpMailer)
//	manager.Register("log", logMailer)
//	manager.SetDefaultMailer("smtp")
//
//	mailer, _ := manager.Mailer("log")
type Manager struct {
	mu      sync.RWMutex
	mailers map[string]Mailer
	def     string
}

// NewManager creates an empty mailer manager.
func NewManager() *Manager {
	return &Manager{mailers: make(map[string]Mailer)}
}

// Register adds a named mailer; the first registered becomes the
// default.
func (m *Manager) Register(name string, mailer Mailer) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.mailers[name] = mailer
	if m.def == "" {
		m.def = name
	}
}

// SetDefaultMailer selects the default mailer by name.
func (m *Manager) SetDefaultMailer(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.def = name
}

// Mailer returns a mailer by name (the default when no name is given).
func (m *Manager) Mailer(name ...string) (Mailer, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	target := m.def
	if len(name) > 0 && name[0] != "" {
		target = name[0]
	}
	mailer, ok := m.mailers[target]
	if !ok {
		return nil, fmt.Errorf("mail: mailer %q is not registered", target)
	}
	return mailer, nil
}

// Send delivers through the default mailer.
func (m *Manager) Send(message *Message) error {
	mailer, err := m.Mailer()
	if err != nil {
		return err
	}
	return mailer.Send(message)
}

// FailoverMailer tries each mailer in order until one delivers -
// Laravel's failover transport:
//
//	mailer := mail.NewFailoverMailer(primarySMTP, backupSMTP, logMailer)
type FailoverMailer struct {
	mailers []Mailer
}

// NewFailoverMailer creates a failover chain.
func NewFailoverMailer(mailers ...Mailer) *FailoverMailer {
	return &FailoverMailer{mailers: mailers}
}

// Send tries each mailer in order, returning nil on the first success
// and the joined errors when every mailer fails.
func (f *FailoverMailer) Send(message *Message) error {
	if len(f.mailers) == 0 {
		return fmt.Errorf("mail: failover mailer has no mailers")
	}
	var errs []error
	for _, mailer := range f.mailers {
		if err := mailer.Send(message); err != nil {
			errs = append(errs, err)
			continue
		}
		return nil
	}
	return fmt.Errorf("mail: all failover mailers failed: %w", errors.Join(errs...))
}
