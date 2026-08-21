// Package seed provides database seeding, mirroring Laravel's db:seed.
package seed

import (
	"fmt"
	"sync"
)

// Seeder populates the database with records.
type Seeder interface {
	// Run executes the seeder.
	Run() error
}

// SeederFunc adapts a function to the Seeder interface.
type SeederFunc func() error

// Run executes the seeder function.
func (f SeederFunc) Run() error { return f() }

type entry struct {
	name   string
	seeder Seeder
}

// Runner holds the application's named seeders and runs them in
// registration order.
type Runner struct {
	seeders []entry
	mu      sync.Mutex
}

// NewRunner creates an empty seeder runner.
func NewRunner() *Runner {
	return &Runner{}
}

// Add registers a named seeder.
func (r *Runner) Add(name string, seeder Seeder) *Runner {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seeders = append(r.seeders, entry{name: name, seeder: seeder})
	return r
}

// AddFunc registers a named seeder function.
func (r *Runner) AddFunc(name string, fn func() error) *Runner {
	return r.Add(name, SeederFunc(fn))
}

// Names returns the registered seeder names in order.
func (r *Runner) Names() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	names := make([]string, len(r.seeders))
	for i, e := range r.seeders {
		names[i] = e.name
	}
	return names
}

// Run executes all seeders, or only the named ones when given.
func (r *Runner) Run(names ...string) error {
	r.mu.Lock()
	seeders := append([]entry(nil), r.seeders...)
	r.mu.Unlock()

	if len(names) > 0 {
		wanted := make(map[string]bool, len(names))
		for _, name := range names {
			wanted[name] = true
		}
		filtered := seeders[:0]
		for _, e := range seeders {
			if wanted[e.name] {
				filtered = append(filtered, e)
				delete(wanted, e.name)
			}
		}
		for name := range wanted {
			return fmt.Errorf("seed: seeder [%s] is not registered", name)
		}
		seeders = filtered
	}

	for _, e := range seeders {
		if err := e.seeder.Run(); err != nil {
			return fmt.Errorf("seed: seeder [%s] failed: %w", e.name, err)
		}
	}
	return nil
}
