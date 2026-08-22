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

	"github.com/genesysflow/go-genesys/support"
)

// Config configures the view manager.
type Config struct {
	// Path is the root views directory (default "resources/views").
	Path string `yaml:"path" json:"path"`

	// Reload re-parses templates on every render; enable in development
	// so edits show up without restarting.
	Reload bool `yaml:"reload" json:"reload"`

	// Layout is the view every page is wrapped in ("layouts.app"),
	// rendering it through {{.content}}. Empty renders pages on their
	// own.
	Layout string `yaml:"layout" json:"layout"`
}

// Manager loads and renders templates.
type Manager struct {
	path      string
	reload    bool
	shared    map[string]any
	funcs     template.FuncMap
	tmpl      *template.Template
	composers []composer
	mu        sync.RWMutex

	// layout is the view every rendered page is wrapped in, unless the
	// render names another or opts out.
	layout string

	// Helper wiring, set by the service providers.
	urls       URLGenerator
	baseURL    string
	config     ConfigResolver
	translator TranslatorFunc
}

// composer fills in data for the views matching a pattern before they
// render, Laravel's view composers.
type composer struct {
	pattern string
	fn      func(data map[string]any)
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
	m := &Manager{
		path:   cfg.Path,
		reload: cfg.Reload,
		layout: cfg.Layout,
		shared: make(map[string]any),
		funcs:  defaultFuncs(),
	}
	for name, fn := range m.helperFuncs() {
		m.funcs[name] = fn
	}

	return m
}

// Composer registers a function that fills in data for every view whose
// name matches the pattern, Laravel's view composers:
//
//	manager.Composer("partials.*", func(data map[string]any) {
//	    data["unread"] = unreadCount()
//	})
//
// A trailing `*` matches any suffix, and "*" matches every view.
// Composers run in registration order, before the handler's own data is
// layered on top - so a composer supplies defaults and never overrides
// what the handler passed.
func (m *Manager) Composer(pattern string, fn func(data map[string]any)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.composers = append(m.composers, composer{pattern: pattern, fn: fn})
}

// composedData runs the composers matching name over a fresh map.
func (m *Manager) composedData(name string) map[string]any {
	m.mu.RLock()
	matching := make([]func(map[string]any), 0, len(m.composers))
	for _, c := range m.composers {
		if matchViewName(c.pattern, name) {
			matching = append(matching, c.fn)
		}
	}
	m.mu.RUnlock()

	if len(matching) == 0 {
		return nil
	}

	// A fresh map per render: a shared one would carry one request's
	// values into the next.
	data := make(map[string]any)
	for _, fn := range matching {
		fn(data)
	}
	return data
}

// matchViewName matches a view name against a composer pattern, where a
// trailing `*` stands for any suffix.
func matchViewName(pattern, name string) bool {
	if pattern == "*" || pattern == name {
		return true
	}
	if prefix, found := strings.CutSuffix(pattern, "*"); found {
		return strings.HasPrefix(name, prefix)
	}
	return false
}

// defaultFuncs are helpers available in every template.
func defaultFuncs() template.FuncMap {
	return template.FuncMap{
		"raw": func(s string) template.HTML {
			return template.HTML(s) // #nosec G203 -- explicit opt-in for trusted HTML
		},
		"upper": strings.ToUpper,
		"lower": strings.ToLower,
		"title": support.Title,
		// dict builds a map inline, for passing data to components:
		// {{component "alert" (dict "type" "error" "message" .err)}}
		"dict": func(pairs ...any) (map[string]any, error) {
			if len(pairs)%2 != 0 {
				return nil, fmt.Errorf("dict: odd number of arguments")
			}
			d := make(map[string]any, len(pairs)/2)
			for i := 0; i < len(pairs); i += 2 {
				key, ok := pairs[i].(string)
				if !ok {
					return nil, fmt.Errorf("dict: keys must be strings, got %T", pairs[i])
				}
				d[key] = pairs[i+1]
			}
			return d, nil
		},
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
	funcs := make(template.FuncMap, len(m.funcs)+2)
	for name, fn := range m.funcs {
		funcs[name] = fn
	}
	// component renders a view under components/ as a reusable partial,
	// Blade components without the compiler:
	//
	//	{{component "alert" (dict "type" "error" "slot" "Something broke")}}
	//
	// The component template reads its data as usual ({{.type}}) and
	// injects slot content with {{raw .slot}} (or {{slot .}}).
	funcs["component"] = func(name string, data ...map[string]any) (template.HTML, error) {
		var payload map[string]any
		if len(data) > 0 {
			payload = data[0]
		}
		out, err := m.RenderString("components."+name, payload)
		if err != nil {
			return "", err
		}
		return template.HTML(out), nil // #nosec G203 -- component output is template-rendered
	}
	// slot renders a component's slot content.
	//
	// A slot is usually filled from a handler, which means from user
	// data, so a plain string is escaped like any other value. Markup
	// says so by arriving as template.HTML - what `raw` produces, and
	// what a nested component already returns.
	funcs["slot"] = func(data map[string]any) template.HTML {
		if data == nil {
			return ""
		}
		switch content := data["slot"].(type) {
		case template.HTML:
			return content
		case string:
			return template.HTML(template.HTMLEscapeString(content)) // #nosec G203 -- escaped on the line above
		case nil:
			return ""
		default:
			return template.HTML(template.HTMLEscapeString(fmt.Sprint(content))) // #nosec G203 -- escaped on the line above
		}
	}

	root := template.New("").Funcs(funcs)

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

// SetLayout sets the view every rendered page is wrapped in - Laravel's
// @extends, without a directive in every file.
//
// The layout is an ordinary view that renders the page through
// {{.content}}:
//
//	<html><body><main>{{.content}}</main></body></html>
//
// RenderIn overrides it for one render, and an empty layout there opts
// out, for a partial or an email body. Rendering the layout itself never
// wraps it in itself.
func (m *Manager) SetLayout(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.layout = name
}

// Layout returns the configured default layout, empty when none is set.
func (m *Manager) Layout() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.layout
}

// Render writes the named view to w, wrapped in the configured layout,
// with the given data merged over shared data.
func (m *Manager) Render(w io.Writer, name string, data map[string]any) error {
	return m.RenderIn(w, m.Layout(), name, data)
}

// RenderIn writes the named view to w inside a specific layout. An empty
// layout renders the view on its own.
func (m *Manager) RenderIn(w io.Writer, layout, name string, data map[string]any) error {
	// A layout rendered directly is the page, not its own wrapper.
	if layout == "" || layout == name {
		return m.renderOne(w, name, data)
	}

	tmpl, err := m.ensureLoaded()
	if err != nil {
		return err
	}
	if tmpl.Lookup(layout) == nil {
		return fmt.Errorf("view: layout [%s] not found in %s", layout, m.path)
	}

	var page strings.Builder
	if err := m.renderOne(&page, name, data); err != nil {
		return err
	}

	// The page is already-rendered HTML: marking it as such is what
	// keeps the layout from escaping the markup it is wrapping.
	wrapped := make(map[string]any, len(data)+1)
	for key, value := range data {
		wrapped[key] = value
	}
	wrapped["content"] = template.HTML(page.String()) //nolint:gosec // rendered by this manager

	return m.renderOne(w, layout, wrapped)
}

// RenderStringIn renders a view inside a layout and returns the result.
func (m *Manager) RenderStringIn(layout, name string, data map[string]any) (string, error) {
	var sb strings.Builder
	if err := m.RenderIn(&sb, layout, name, data); err != nil {
		return "", err
	}
	return sb.String(), nil
}

// renderOne writes one view with no layout around it.
func (m *Manager) renderOne(w io.Writer, name string, data map[string]any) error {
	tmpl, err := m.ensureLoaded()
	if err != nil {
		return err
	}
	target := tmpl.Lookup(name)
	if target == nil {
		return fmt.Errorf("view: view [%s] not found in %s", name, m.path)
	}

	composed := m.composedData(name)

	m.mu.RLock()
	merged := make(map[string]any, len(m.shared)+len(composed)+len(data))
	for k, v := range m.shared {
		merged[k] = v
	}
	m.mu.RUnlock()
	for k, v := range composed {
		merged[k] = v
	}
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
