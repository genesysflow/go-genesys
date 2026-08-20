// Package lang provides Laravel-style localization. Translation files live
// under a lang directory, one folder per locale:
//
//	lang/
//	├── en/
//	│   └── messages.yaml   # welcome: "Welcome, :name!"
//	└── es/
//	    └── messages.yaml   # welcome: "¡Bienvenido, :name!"
//
// Keys use dot notation with the file name as prefix: "messages.welcome".
package lang

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// Translator resolves translation keys for a locale with a fallback.
type Translator struct {
	path     string
	locale   string
	fallback string

	lines map[string]map[string]string // locale -> flattened key -> message
	mu    sync.RWMutex
}

// Config configures the translator.
type Config struct {
	// Path is the lang directory (default "lang").
	Path string `yaml:"path" json:"path"`

	// Locale is the active locale (default "en").
	Locale string `yaml:"locale" json:"locale"`

	// Fallback is the locale used when the active one misses a key
	// (default "en").
	Fallback string `yaml:"fallback" json:"fallback"`
}

// New creates a translator.
func New(config ...Config) *Translator {
	cfg := Config{}
	if len(config) > 0 {
		cfg = config[0]
	}
	if cfg.Path == "" {
		cfg.Path = "lang"
	}
	if cfg.Locale == "" {
		cfg.Locale = "en"
	}
	if cfg.Fallback == "" {
		cfg.Fallback = "en"
	}
	return &Translator{
		path:     cfg.Path,
		locale:   cfg.Locale,
		fallback: cfg.Fallback,
		lines:    make(map[string]map[string]string),
	}
}

// SetLocale changes the active locale.
func (t *Translator) SetLocale(locale string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.locale = locale
}

// Locale returns the active locale.
func (t *Translator) Locale() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.locale
}

// AddLine registers a translation programmatically (useful in tests).
func (t *Translator) AddLine(locale, key, message string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.lines[locale] == nil {
		t.lines[locale] = make(map[string]string)
	}
	t.lines[locale][key] = message
}

// loadLocale loads a locale's files once.
func (t *Translator) loadLocale(locale string) map[string]string {
	t.mu.RLock()
	lines, loaded := t.lines[locale]
	t.mu.RUnlock()
	if loaded {
		return lines
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	if lines, loaded := t.lines[locale]; loaded {
		return lines
	}

	lines = make(map[string]string)
	dir := filepath.Join(t.path, locale)
	entries, err := os.ReadDir(dir)
	if err == nil {
		for _, entry := range entries {
			ext := filepath.Ext(entry.Name())
			if entry.IsDir() || (ext != ".yaml" && ext != ".yml" && ext != ".json") {
				continue
			}
			content, err := os.ReadFile(filepath.Join(dir, entry.Name()))
			if err != nil {
				continue
			}
			var parsed map[string]any
			if err := yaml.Unmarshal(content, &parsed); err != nil {
				continue
			}
			prefix := strings.TrimSuffix(entry.Name(), ext)
			flatten(prefix, parsed, lines)
		}
	}
	t.lines[locale] = lines
	return lines
}

// flatten converts nested maps into dot-notation keys.
func flatten(prefix string, value map[string]any, out map[string]string) {
	for key, raw := range value {
		full := prefix + "." + key
		switch v := raw.(type) {
		case map[string]any:
			flatten(full, v, out)
		default:
			out[full] = fmt.Sprint(v)
		}
	}
}

// lookup finds a key in the active locale, then the fallback.
func (t *Translator) lookup(key string) (string, bool) {
	if message, ok := t.loadLocale(t.Locale())[key]; ok {
		return message, true
	}
	if fallback := t.fallback; fallback != t.Locale() {
		if message, ok := t.loadLocale(fallback)[key]; ok {
			return message, true
		}
	}
	return "", false
}

// Trans translates a key, applying :placeholder replacements. Missing keys
// return the key itself, like Laravel.
func (t *Translator) Trans(key string, replacements ...map[string]string) string {
	message, ok := t.lookup(key)
	if !ok {
		return key
	}
	return replacePlaceholders(message, replacements...)
}

// Has reports whether a key exists in the active locale or fallback.
func (t *Translator) Has(key string) bool {
	_, ok := t.lookup(key)
	return ok
}

// TransChoice translates a pluralizable key ("one apple|:count apples"),
// choosing the segment by count and replacing :count automatically.
func (t *Translator) TransChoice(key string, count int, replacements ...map[string]string) string {
	message, ok := t.lookup(key)
	if !ok {
		return key
	}

	parts := strings.Split(message, "|")
	chosen := parts[0]
	if count != 1 && len(parts) > 1 {
		chosen = parts[1]
	}

	all := map[string]string{"count": fmt.Sprint(count)}
	for _, extra := range replacements {
		for k, v := range extra {
			all[k] = v
		}
	}
	return replacePlaceholders(chosen, all)
}

// replacePlaceholders substitutes :name style placeholders. Keys are
// applied longest-first (like Laravel) so ":name" cannot corrupt
// ":name_full" when both are present.
func replacePlaceholders(message string, replacements ...map[string]string) string {
	merged := map[string]string{}
	for _, set := range replacements {
		for key, value := range set {
			merged[key] = value
		}
	}
	keys := make([]string, 0, len(merged))
	for key := range merged {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if len(keys[i]) != len(keys[j]) {
			return len(keys[i]) > len(keys[j])
		}
		return keys[i] < keys[j]
	})
	for _, key := range keys {
		message = strings.ReplaceAll(message, ":"+key, merged[key])
	}
	return message
}

// lookupIn finds a key in a specific locale, then the fallback.
func (t *Translator) lookupIn(locale, key string) (string, bool) {
	if message, ok := t.loadLocale(locale)[key]; ok {
		return message, true
	}
	if fallback := t.fallback; fallback != locale {
		if message, ok := t.loadLocale(fallback)[key]; ok {
			return message, true
		}
	}
	return "", false
}

// LocaleView resolves translations in one fixed locale, so a request
// can translate in the visitor's language without mutating the shared
// translator's active locale.
type LocaleView struct {
	t      *Translator
	locale string
}

// In returns a view over the translator pinned to the given locale:
//
//	t.In("de").Trans("messages.hi", map[string]string{"name": "Ada"})
func (t *Translator) In(locale string) *LocaleView {
	return &LocaleView{t: t, locale: locale}
}

// Locale returns the view's locale.
func (v *LocaleView) Locale() string { return v.locale }

// Trans translates a key in the view's locale.
func (v *LocaleView) Trans(key string, replacements ...map[string]string) string {
	message, ok := v.t.lookupIn(v.locale, key)
	if !ok {
		return key
	}
	return replacePlaceholders(message, replacements...)
}

// Has reports whether a key exists in the view's locale or fallback.
func (v *LocaleView) Has(key string) bool {
	_, ok := v.t.lookupIn(v.locale, key)
	return ok
}

// TransChoice translates a pluralizable key in the view's locale.
func (v *LocaleView) TransChoice(key string, count int, replacements ...map[string]string) string {
	message, ok := v.t.lookupIn(v.locale, key)
	if !ok {
		return key
	}
	parts := strings.Split(message, "|")
	chosen := parts[0]
	if count != 1 && len(parts) > 1 {
		chosen = parts[1]
	}
	all := map[string]string{"count": fmt.Sprint(count)}
	for _, extra := range replacements {
		for k, v := range extra {
			all[k] = v
		}
	}
	return replacePlaceholders(chosen, all)
}
