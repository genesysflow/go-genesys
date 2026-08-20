// Package view provides a Laravel-style view layer on html/template.
//
// Templates live in a views directory and are addressed with dot notation:
// resources/views/users/index.html renders as "users.index". Templates can
// compose each other with {{template "layouts.app" .}}.
package view

import (
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Config configures the view manager.
type Config struct {
	// Path is the root views directory (default "resources/views").
	Path string `yaml:"path" json:"path"`

	// Reload re-parses templates on every render; enable in development
	// so edits show up without restarting.
	Reload bool `yaml:"reload" json:"reload"`
}

// Manager loads and renders templates.
type Manager struct {
	path   string
	reload bool
	shared map[string]any
	funcs  template.FuncMap
	tmpl   *template.Template
	mu     sync.RWMutex
}

// NewManager creates a view manager for the given configuration.
func NewManager(config ...Config) *Manager {
	cfg := Config{}
	if len(config) > 0 {
		cfg = config[0]
	}
	if cfg.Path == "" {
		cfg.Path = "resources/views"
	}
	return &Manager{
		path:   cfg.Path,
		reload: cfg.Reload,
		shared: make(map[string]any),
		funcs:  defaultFuncs(),
	}
}

// defaultFuncs are helpers available in every template.
func defaultFuncs() template.FuncMap {
	return template.FuncMap{
		"raw": func(s string) template.HTML {
			return template.HTML(s) // #nosec G203 -- explicit opt-in for trusted HTML
		},
		"upper": strings.ToUpper,
		"lower": strings.ToLower,
		"title": strings.Title,
	}
}

// AddFunc registers a template function; call before the first render.
func (m *Manager) AddFunc(name string, fn any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.funcs[name] = fn
	m.tmpl = nil // force re-parse
}

// Share makes a value available to every template as {{.key}}.
func (m *Manager) Share(key string, value any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.shared[key] = value
}

// viewExtensions are the file extensions treated as templates.
var viewExtensions = map[string]bool{
	".html":   true,
	".gohtml": true,
	".tmpl":   true,
}

// Load parses all templates under the views directory.
func (m *Manager) Load() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.loadLocked()
}

func (m *Manager) loadLocked() error {
	root := template.New("").Funcs(m.funcs)

	err := filepath.WalkDir(m.path, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !viewExtensions[filepath.Ext(path)] {
			return nil
		}

		rel, err := filepath.Rel(m.path, path)
		if err != nil {
			return err
		}
		name := viewName(rel)

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if _, err := root.New(name).Parse(string(content)); err != nil {
			return fmt.Errorf("view: parse %s: %w", rel, err)
		}
		return nil
	})
	if err != nil {
		return err
	}

	m.tmpl = root
	return nil
}

// viewName converts a relative path to a dot-notation view name.
func viewName(rel string) string {
	rel = filepath.ToSlash(rel)
	rel = strings.TrimSuffix(rel, filepath.Ext(rel))
	return strings.ReplaceAll(rel, "/", ".")
}

// ensureLoaded parses templates on first use (and every render when
// reloading is enabled).
func (m *Manager) ensureLoaded() (*template.Template, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.tmpl == nil || m.reload {
		if err := m.loadLocked(); err != nil {
			return nil, err
		}
	}
	return m.tmpl, nil
}

// Exists reports whether a view with the given name is loaded.
func (m *Manager) Exists(name string) bool {
	tmpl, err := m.ensureLoaded()
	if err != nil {
		return false
	}
	return tmpl.Lookup(name) != nil
}

// Render writes the named view to w with the given data merged over
// shared data.
func (m *Manager) Render(w io.Writer, name string, data map[string]any) error {
	tmpl, err := m.ensureLoaded()
	if err != nil {
		return err
	}
	target := tmpl.Lookup(name)
	if target == nil {
		return fmt.Errorf("view: view [%s] not found in %s", name, m.path)
	}

	m.mu.RLock()
	merged := make(map[string]any, len(m.shared)+len(data))
	for k, v := range m.shared {
		merged[k] = v
	}
	m.mu.RUnlock()
	for k, v := range data {
		merged[k] = v
	}

	return target.Execute(w, merged)
}

// RenderString renders the named view to a string.
func (m *Manager) RenderString(name string, data map[string]any) (string, error) {
	var sb strings.Builder
	if err := m.Render(&sb, name, data); err != nil {
		return "", err
	}
	return sb.String(), nil
}
