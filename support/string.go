package support

import "strings"

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
