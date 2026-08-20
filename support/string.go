package support

import (
	"strings"
	"unicode"
)

// ToPascalCase converts a string to PascalCase. Mixed-case words keep
// their interior capitals, so already-camelled input survives:
// "send_email" -> "SendEmail", "sendEmail" -> "SendEmail",
// "SendEmail" -> "SendEmail", "CREATE_USERS" -> "CreateUsers".
func ToPascalCase(s string) string {
	words := strings.FieldsFunc(s, func(r rune) bool {
		return r == '_' || r == '-' || r == ' '
	})

	for i, word := range words {
		if word == strings.ToLower(word) || word == strings.ToUpper(word) {
			// Single-case words get classic studly treatment.
			words[i] = strings.ToUpper(string(word[0])) + strings.ToLower(word[1:])
			continue
		}
		// Mixed case: preserve the author's camelling, just capitalise.
		words[i] = strings.ToUpper(string(word[0])) + word[1:]
	}

	return strings.Join(words, "")
}

// Title converts a string to Title Case: the first letter of each word
// is uppercased and the rest lowercased, like Laravel's Str::title
// ("hello WORLD" -> "Hello World"). Letters and digits continue a word
// ("x1y" -> "X1y"); everything else ends one, and spacing is preserved.
func Title(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	inWord := false
	for _, r := range s {
		switch {
		case unicode.IsLetter(r):
			if inWord {
				b.WriteRune(unicode.ToLower(r))
			} else {
				b.WriteRune(unicode.ToUpper(r))
			}
			inWord = true
		case unicode.IsDigit(r):
			b.WriteRune(r)
			inWord = true
		default:
			b.WriteRune(r)
			inWord = false
		}
	}
	return b.String()
}

// ToSnakeCase converts a string to snake_case.
func ToSnakeCase(s string) string {
	var result strings.Builder
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			result.WriteByte('_')
		}
		result.WriteRune(r)
	}
	return strings.ToLower(result.String())
}
