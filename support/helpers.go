// Package support provides helper utilities for the framework.
package support

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// Str provides string helper functions.
var Str = &StringHelper{}

// StringHelper contains string manipulation methods.
type StringHelper struct{}

// Random generates a cryptographically random hex string of the given length
// in characters.
//
// It draws a full byte of entropy per output character rather than encoding
// length bytes and truncating the result: hex doubles its input, so the
// obvious implementation silently yields half the entropy the caller asked
// for — Random(32) would be 128 bits, not 256.
func (s *StringHelper) Random(length int) string {
	if length <= 0 {
		return ""
	}
	// One hex character encodes 4 bits, so ceil(length/2) bytes cover it.
	buf := make([]byte, (length+1)/2)
	mustReadRandom(buf)
	return hex.EncodeToString(buf)[:length]
}

// UUID generates a UUID v4 string.
func (s *StringHelper) UUID() string {
	uuid := make([]byte, 16)
	mustReadRandom(uuid)
	uuid[6] = (uuid[6] & 0x0f) | 0x40
	uuid[8] = (uuid[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:])
}

// mustReadRandom fills buf from the system CSPRNG, panicking if it cannot.
//
// A silent failure here would hand out predictable tokens and session
// identifiers, so refusing to continue is the correct response — there is no
// safe degraded mode for a random source.
func mustReadRandom(buf []byte) {
	if _, err := rand.Read(buf); err != nil {
		panic("support: crypto/rand unavailable: " + err.Error())
	}
}

// Slug generates a URL-friendly slug from a string.
func (s *StringHelper) Slug(str string) string {
	str = strings.ToLower(str)
	reg := regexp.MustCompile(`[^a-z0-9]+`)
	str = reg.ReplaceAllString(str, "-")
	str = strings.Trim(str, "-")
	return str
}

// Camel converts a string to camelCase.
func (s *StringHelper) Camel(str string) string {
	words := s.words(str)
	if len(words) == 0 {
		return ""
	}
	caser := cases.Title(language.English)
	words[0] = strings.ToLower(words[0])
	for i := 1; i < len(words); i++ {
		words[i] = caser.String(strings.ToLower(words[i]))
	}
	return strings.Join(words, "")
}

// Pascal converts a string to PascalCase.
func (s *StringHelper) Pascal(str string) string {
	words := s.words(str)
	caser := cases.Title(language.English)
	for i := range words {
		words[i] = caser.String(strings.ToLower(words[i]))
	}
	return strings.Join(words, "")
}

// Snake converts a string to snake_case.
func (s *StringHelper) Snake(str string) string {
	words := s.words(str)
	for i := range words {
		words[i] = strings.ToLower(words[i])
	}
	return strings.Join(words, "_")
}

// Kebab converts a string to kebab-case.
func (s *StringHelper) Kebab(str string) string {
	words := s.words(str)
	for i := range words {
		words[i] = strings.ToLower(words[i])
	}
	return strings.Join(words, "-")
}

// Title converts a string to Title Case.
func (s *StringHelper) Title(str string) string {
	caser := cases.Title(language.English)
	return caser.String(strings.ToLower(str))
}

// Lower converts a string to lowercase.
func (s *StringHelper) Lower(str string) string {
	return strings.ToLower(str)
}

// Upper converts a string to uppercase.
func (s *StringHelper) Upper(str string) string {
	return strings.ToUpper(str)
}

// Limit truncates a string to the given number of characters (runes,
// not bytes, so multibyte text is never cut mid-character).
func (s *StringHelper) Limit(str string, limit int, end ...string) string {
	suffix := "..."
	if len(end) > 0 {
		suffix = end[0]
	}
	if limit < 0 {
		limit = 0
	}
	runes := []rune(str)
	if len(runes) <= limit {
		return str
	}
	return string(runes[:limit]) + suffix
}

// Contains checks if a string contains a substring.
func (s *StringHelper) Contains(str, substr string) bool {
	return strings.Contains(str, substr)
}

// StartsWith checks if a string starts with a prefix.
func (s *StringHelper) StartsWith(str, prefix string) bool {
	return strings.HasPrefix(str, prefix)
}

// EndsWith checks if a string ends with a suffix.
func (s *StringHelper) EndsWith(str, suffix string) bool {
	return strings.HasSuffix(str, suffix)
}

// Trim trims whitespace from a string.
func (s *StringHelper) Trim(str string) string {
	return strings.TrimSpace(str)
}

// Replace replaces occurrences in a string.
func (s *StringHelper) Replace(str, old, new string) string {
	return strings.ReplaceAll(str, old, new)
}

// words splits a string into words.
func (s *StringHelper) words(str string) []string {
	var words []string
	var word strings.Builder
	for i, r := range str {
		if unicode.IsUpper(r) && i > 0 {
			if word.Len() > 0 {
				words = append(words, word.String())
				word.Reset()
			}
		}
		if r == '_' || r == '-' || r == ' ' {
			if word.Len() > 0 {
				words = append(words, word.String())
				word.Reset()
			}
			continue
		}
		word.WriteRune(r)
	}
	if word.Len() > 0 {
		words = append(words, word.String())
	}
	return words
}

// Arr provides array/slice helper functions.
var Arr = &ArrayHelper{}

// ArrayHelper contains array manipulation methods.
type ArrayHelper struct{}

// Contains checks if an array contains a value.
func (a *ArrayHelper) Contains(slice any, item any) bool {
	s := reflect.ValueOf(slice)
	if s.Kind() != reflect.Slice {
		return false
	}
	for i := 0; i < s.Len(); i++ {
		if reflect.DeepEqual(s.Index(i).Interface(), item) {
			return true
		}
	}
	return false
}

// First returns the first element of a slice.
func (a *ArrayHelper) First(slice any) any {
	s := reflect.ValueOf(slice)
	if s.Kind() != reflect.Slice || s.Len() == 0 {
		return nil
	}
	return s.Index(0).Interface()
}

// Last returns the last element of a slice.
func (a *ArrayHelper) Last(slice any) any {
	s := reflect.ValueOf(slice)
	if s.Kind() != reflect.Slice || s.Len() == 0 {
		return nil
	}
	return s.Index(s.Len() - 1).Interface()
}

// Path provides path helper functions.
var Path = &PathHelper{}

// PathHelper contains path manipulation methods.
type PathHelper struct{}

// Join joins path elements.
func (p *PathHelper) Join(elem ...string) string {
	return filepath.Join(elem...)
}

// Base returns the base name of a path.
func (p *PathHelper) Base(path string) string {
	return filepath.Base(path)
}

// Dir returns the directory of a path.
func (p *PathHelper) Dir(path string) string {
	return filepath.Dir(path)
}

// Ext returns the file extension.
func (p *PathHelper) Ext(path string) string {
	return filepath.Ext(path)
}

// Exists checks if a path exists.
func (p *PathHelper) Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// IsDir checks if a path is a directory.
func (p *PathHelper) IsDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// IsFile checks if a path is a file.
func (p *PathHelper) IsFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// Now returns the current time.
func Now() time.Time {
	return time.Now()
}

// Sleep pauses for the given duration.
func Sleep(d time.Duration) {
	time.Sleep(d)
}

// RandomBytes generates n cryptographically random bytes.
func RandomBytes(n int) ([]byte, error) {
	bytes := make([]byte, n)
	_, err := rand.Read(bytes)
	return bytes, err
}

// RandomString generates a cryptographically random URL-safe string of n
// characters.
//
// As with Random, entropy is sized from the requested output length rather
// than encoding n bytes and cutting the result short, which would discard a
// quarter of the entropy the caller expected.
func RandomString(n int) string {
	if n <= 0 {
		return ""
	}
	// Each base64 character carries 6 bits, so ceil(n*6/8) bytes cover it.
	buf := make([]byte, (n*6+7)/8)
	mustReadRandom(buf)
	return base64.RawURLEncoding.EncodeToString(buf)[:n]
}

// Tap calls the given function and returns the value.
func Tap[T any](value T, fn func(T)) T {
	fn(value)
	return value
}

// With calls the given function with the value and returns the result.
func With[T any, R any](value T, fn func(T) R) R {
	return fn(value)
}

// When conditionally executes a function.
func When[T any](condition bool, value T, fn func(T) T) T {
	if condition {
		return fn(value)
	}
	return value
}

// Unless conditionally executes a function when condition is false.
func Unless[T any](condition bool, value T, fn func(T) T) T {
	if !condition {
		return fn(value)
	}
	return value
}

// Retry retries a function until it succeeds or max attempts is reached.
func Retry(attempts int, sleep time.Duration, fn func() error) error {
	var err error
	for i := 0; i < attempts; i++ {
		err = fn()
		if err == nil {
			return nil
		}
		if i < attempts-1 {
			time.Sleep(sleep)
		}
	}
	return err
}
