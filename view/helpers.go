package view

import (
	"fmt"
	"html/template"
	"strings"
)

// URLGenerator resolves a named route to a path, matching the router's
// URL method.
type URLGenerator func(name string, params ...map[string]any) string

// ConfigResolver reads an application configuration value.
type ConfigResolver func(key string) any

// TranslatorFunc translates a key, matching the translator's Trans.
type TranslatorFunc func(key string, replacements ...map[string]string) string

// SetURLGenerator wires the `route` helper to the application router:
//
//	<a href="{{route "users.show" (dict "user" .user.ID)}}">
//
// Until it is set, `route` renders an empty string rather than failing to
// parse - an unregistered template function takes down every view, not
// just the one using it.
func (m *Manager) SetURLGenerator(fn URLGenerator) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.urls = fn
}

// SetBaseURL sets the application URL the `url` and `asset` helpers build
// on. With no base URL both fall back to a root-relative path.
func (m *Manager) SetBaseURL(base string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.baseURL = strings.TrimSuffix(base, "/")
}

// SetConfigResolver wires the `config` helper to application config.
func (m *Manager) SetConfigResolver(fn ConfigResolver) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.config = fn
}

// SetTranslator wires the `trans` helper to the translator. Until it is
// set, `trans` renders the key itself.
func (m *Manager) SetTranslator(fn TranslatorFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.translator = fn
}

// helperFuncs are the request-independent helpers every template gets.
// They read their wiring at render time, so a provider may set it after
// templates are parsed.
func (m *Manager) helperFuncs() template.FuncMap {
	return template.FuncMap{
		"route": func(name string, params ...map[string]any) string {
			m.mu.RLock()
			generate := m.urls
			m.mu.RUnlock()
			if generate == nil {
				return ""
			}
			return generate(name, params...)
		},

		"url": func(path string) string {
			return m.absoluteURL(path)
		},

		"asset": func(path string) string {
			return m.absoluteURL(path)
		},

		"config": func(key string) any {
			m.mu.RLock()
			resolve := m.config
			m.mu.RUnlock()
			if resolve == nil {
				return nil
			}
			return resolve(key)
		},

		"trans": func(key string, replacements ...map[string]string) string {
			m.mu.RLock()
			translate := m.translator
			m.mu.RUnlock()
			if translate == nil {
				return key
			}
			return translate(key, replacements...)
		},

		"csrf_field":   csrfField,
		"method_field": methodField,
	}
}

// absoluteURL joins path onto the base URL, keeping it root-relative when
// no base URL is configured.
func (m *Manager) absoluteURL(path string) string {
	m.mu.RLock()
	base := m.baseURL
	m.mu.RUnlock()

	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return base + path
}

// csrfField renders the hidden input a form posts its CSRF token in,
// Laravel's @csrf. The token comes from the request:
//
//	<form method="POST">{{csrf_field .csrf_token}}
func csrfField(token string) template.HTML {
	return template.HTML(fmt.Sprintf( // #nosec G203 -- token is escaped below
		`<input type="hidden" name="_token" value="%s">`,
		template.HTMLEscapeString(token),
	))
}

// spoofableMethods are the verbs a form may spoof through _method. A form
// can only be submitted as GET or POST, so anything else has to be
// declared this way.
var spoofableMethods = map[string]bool{
	"PUT":    true,
	"PATCH":  true,
	"DELETE": true,
}

// SpoofedMethod normalises a verb declared by a form and reports whether
// a form may legitimately declare it.
//
// It exists so that the side writing _method and the side honouring it
// cannot disagree. A renderer that emits a verb the server will not act
// on produces a button that answers 405, and a server that acts on a
// verb no form may write is a request-forging surface; both are the same
// bug, and this is the one place that decides.
//
// The allowlist is deliberately narrow. GET is not on it: a link, a
// prefetch or a crawler must never be able to perform a destructive
// action, and a spoofed GET is exactly that.
func SpoofedMethod(declared string) (string, bool) {
	verb := strings.ToUpper(strings.TrimSpace(declared))
	return verb, spoofableMethods[verb]
}

// methodField renders the hidden input that spoofs the request method,
// Laravel's @method. Anything that is not a spoofable verb renders
// nothing rather than smuggling its content into the form.
func methodField(method string) template.HTML {
	verb, ok := SpoofedMethod(method)
	if !ok {
		return ""
	}
	return template.HTML(fmt.Sprintf( // #nosec G203 -- verb is from a fixed allowlist
		`<input type="hidden" name="_method" value="%s">`,
		verb,
	))
}
