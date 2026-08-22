package support

import (
	"strings"
	"unicode"

	"github.com/jinzhu/inflection"
)

// Plural returns the plural form of a word ("user" -> "users").
func (s *StringHelper) Plural(word string) string {
	return inflection.Plural(word)
}

// Singular returns the singular form of a word ("users" -> "user").
func (s *StringHelper) Singular(word string) string {
	return inflection.Singular(word)
}

// PluralCount pluralizes a word unless the count is exactly one, which
// is what a message needs: "1 user", "2 users".
func (s *StringHelper) PluralCount(word string, count int) string {
	if count == 1 || count == -1 {
		return inflection.Singular(word)
	}
	return inflection.Plural(word)
}

// Before returns everything before the first occurrence of search. A
// string that does not contain it is returned whole, so a helper never
// silently empties a value.
func (s *StringHelper) Before(str, search string) string {
	if search == "" {
		return str
	}
	if index := strings.Index(str, search); index >= 0 {
		return str[:index]
	}
	return str
}

// BeforeLast returns everything before the last occurrence of search.
func (s *StringHelper) BeforeLast(str, search string) string {
	if search == "" {
		return str
	}
	if index := strings.LastIndex(str, search); index >= 0 {
		return str[:index]
	}
	return str
}

// After returns everything after the first occurrence of search.
func (s *StringHelper) After(str, search string) string {
	if search == "" {
		return str
	}
	if index := strings.Index(str, search); index >= 0 {
		return str[index+len(search):]
	}
	return str
}

// AfterLast returns everything after the last occurrence of search.
func (s *StringHelper) AfterLast(str, search string) string {
	if search == "" {
		return str
	}
	if index := strings.LastIndex(str, search); index >= 0 {
		return str[index+len(search):]
	}
	return str
}

// Between returns what lies between the first from and the following to,
// or an empty string when either is missing.
func (s *StringHelper) Between(str, from, to string) string {
	start := strings.Index(str, from)
	if start < 0 {
		return ""
	}
	rest := str[start+len(from):]

	end := strings.Index(rest, to)
	if end < 0 {
		return ""
	}
	return rest[:end]
}

// Mask replaces characters from an offset with a mask character, for
// showing part of an email or a card number without disclosing it:
//
//	support.Str.Mask("ada@example.com", '*', 4)     // ada@********
//	support.Str.Mask("ada@example.com", '*', 4, 3)  // ada@***le.com
//
// A negative offset counts from the end; masking past the end of the
// string is a no-op.
func (s *StringHelper) Mask(str string, mask rune, index int, length ...int) string {
	runes := []rune(str)
	if index < 0 {
		index = len(runes) + index
	}
	if index < 0 || index >= len(runes) {
		return str
	}

	end := len(runes)
	if len(length) > 0 && length[0] >= 0 && index+length[0] < end {
		end = index + length[0]
	}

	for i := index; i < end; i++ {
		runes[i] = mask
	}
	return string(runes)
}

// Squish collapses every run of whitespace into a single space and trims
// the ends.
func (s *StringHelper) Squish(str string) string {
	return strings.Join(strings.Fields(str), " ")
}

// Ucfirst upper-cases the first character.
func (s *StringHelper) Ucfirst(str string) string {
	if str == "" {
		return str
	}
	runes := []rune(str)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

// Lcfirst lower-cases the first character.
func (s *StringHelper) Lcfirst(str string) string {
	if str == "" {
		return str
	}
	runes := []rune(str)
	runes[0] = unicode.ToLower(runes[0])
	return string(runes)
}

// Studly converts a string to StudlyCase ("foo_bar" -> "FooBar").
func (s *StringHelper) Studly(str string) string {
	return ToPascalCase(str)
}

// Wrap surrounds a string with a delimiter, or with a pair of them.
func (s *StringHelper) Wrap(str, before string, after ...string) string {
	closing := before
	if len(after) > 0 {
		closing = after[0]
	}
	return before + str + closing
}

// Take returns the first n characters, or the last n when n is
// negative. Characters, not bytes: a multi-byte string is never cut
// mid-character.
func (s *StringHelper) Take(str string, n int) string {
	runes := []rune(str)
	if n >= 0 {
		if n > len(runes) {
			n = len(runes)
		}
		return string(runes[:n])
	}

	if -n > len(runes) {
		return str
	}
	return string(runes[len(runes)+n:])
}

// Length returns the number of characters, not bytes.
func (s *StringHelper) Length(str string) int {
	return len([]rune(str))
}

// Of starts a fluent chain over a string, so a transformation reads as
// one statement:
//
//	support.Str.Of("  Hello World  ").Trim().Lower().Replace(" ", "-").Value()
func (s *StringHelper) Of(str string) *Stringable {
	return &Stringable{value: str}
}

// Stringable is a string being transformed by a chain of helpers.
type Stringable struct {
	value string
}

// Value returns the string the chain produced.
func (s *Stringable) Value() string { return s.value }

// String makes a Stringable printable.
func (s *Stringable) String() string { return s.value }

// Length returns the number of characters.
func (s *Stringable) Length() int { return len([]rune(s.value)) }

// IsEmpty reports whether the string is empty.
func (s *Stringable) IsEmpty() bool { return s.value == "" }

// Trim removes surrounding whitespace, or the given cutset.
func (s *Stringable) Trim(cutset ...string) *Stringable {
	if len(cutset) > 0 {
		s.value = strings.Trim(s.value, cutset[0])
		return s
	}
	s.value = strings.TrimSpace(s.value)
	return s
}

// Lower lower-cases the string.
func (s *Stringable) Lower() *Stringable {
	s.value = strings.ToLower(s.value)
	return s
}

// Upper upper-cases the string.
func (s *Stringable) Upper() *Stringable {
	s.value = strings.ToUpper(s.value)
	return s
}

// Title title-cases the string.
func (s *Stringable) Title() *Stringable {
	s.value = Title(s.value)
	return s
}

// Replace substitutes every occurrence of old with new.
func (s *Stringable) Replace(old, new string) *Stringable {
	s.value = strings.ReplaceAll(s.value, old, new)
	return s
}

// Slug renders the string as a URL slug.
func (s *Stringable) Slug() *Stringable {
	s.value = Str.Slug(s.value)
	return s
}

// Camel renders the string in camelCase.
func (s *Stringable) Camel() *Stringable {
	s.value = Str.Camel(s.value)
	return s
}

// Snake renders the string in snake_case.
func (s *Stringable) Snake() *Stringable {
	s.value = ToSnakeCase(s.value)
	return s
}

// Studly renders the string in StudlyCase.
func (s *Stringable) Studly() *Stringable {
	s.value = ToPascalCase(s.value)
	return s
}

// Limit truncates the string, appending an ellipsis (or the given end).
func (s *Stringable) Limit(limit int, end ...string) *Stringable {
	s.value = Str.Limit(s.value, limit, end...)
	return s
}

// Before keeps everything before the first occurrence of search.
func (s *Stringable) Before(search string) *Stringable {
	s.value = Str.Before(s.value, search)
	return s
}

// After keeps everything after the first occurrence of search.
func (s *Stringable) After(search string) *Stringable {
	s.value = Str.After(s.value, search)
	return s
}

// Append adds to the end of the string.
func (s *Stringable) Append(suffix ...string) *Stringable {
	s.value += strings.Join(suffix, "")
	return s
}

// Prepend adds to the start of the string.
func (s *Stringable) Prepend(prefix ...string) *Stringable {
	s.value = strings.Join(prefix, "") + s.value
	return s
}

// Squish collapses whitespace runs into single spaces.
func (s *Stringable) Squish() *Stringable {
	s.value = Str.Squish(s.value)
	return s
}

// Ucfirst upper-cases the first character.
func (s *Stringable) Ucfirst() *Stringable {
	s.value = Str.Ucfirst(s.value)
	return s
}

// Mask replaces characters from an offset with a mask character.
func (s *Stringable) Mask(mask rune, index int, length ...int) *Stringable {
	s.value = Str.Mask(s.value, mask, index, length...)
	return s
}

// Contains reports whether the string contains the substring.
func (s *Stringable) Contains(substr string) bool {
	return strings.Contains(s.value, substr)
}

// StartsWith reports whether the string starts with the prefix.
func (s *Stringable) StartsWith(prefix string) bool {
	return strings.HasPrefix(s.value, prefix)
}

// EndsWith reports whether the string ends with the suffix.
func (s *Stringable) EndsWith(suffix string) bool {
	return strings.HasSuffix(s.value, suffix)
}

// When applies a transformation only while the condition holds, so a
// conditional step stays inside the chain.
func (s *Stringable) When(condition bool, fn func(*Stringable) *Stringable) *Stringable {
	if condition {
		return fn(s)
	}
	return s
}

// Unless is the inverse of When.
func (s *Stringable) Unless(condition bool, fn func(*Stringable) *Stringable) *Stringable {
	return s.When(!condition, fn)
}

// Tap hands the chain to a function without changing it, for logging or
// an assertion mid-chain.
func (s *Stringable) Tap(fn func(*Stringable)) *Stringable {
	fn(s)
	return s
}
