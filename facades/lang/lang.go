// Package lang provides a static facade for the translator.
package lang

import (
	"sync"

	baselang "github.com/genesysflow/go-genesys/lang"
)

var (
	instance *baselang.Translator
	mu       sync.RWMutex
)

// SetInstance sets the translator instance.
// This is called during application bootstrap.
func SetInstance(translator *baselang.Translator) {
	mu.Lock()
	defer mu.Unlock()
	instance = translator
}

// GetInstance returns the translator instance.
func GetInstance() *baselang.Translator {
	mu.RLock()
	defer mu.RUnlock()
	return instance
}

func translator() *baselang.Translator {
	mu.RLock()
	defer mu.RUnlock()
	if instance == nil {
		panic("lang: facade not initialised - register the LangServiceProvider")
	}
	return instance
}

// Trans translates a key with optional :placeholder replacements.
func Trans(key string, replacements ...map[string]string) string {
	return translator().Trans(key, replacements...)
}

// T is shorthand for Trans.
func T(key string, replacements ...map[string]string) string {
	return Trans(key, replacements...)
}

// TransChoice translates a pluralizable key by count.
func TransChoice(key string, count int, replacements ...map[string]string) string {
	return translator().TransChoice(key, count, replacements...)
}

// SetLocale changes the active locale.
func SetLocale(locale string) {
	translator().SetLocale(locale)
}

// Locale returns the active locale.
func Locale() string {
	return translator().Locale()
}
