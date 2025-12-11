package contracts

// Validator defines the interface for input validation.
type Validator interface {
	// Validate validates the given data against the rules.
	Validate(data any) ValidationResult

	// ValidateMap validates a map against the rules.
	ValidateMap(data map[string]any, rules map[string]string) ValidationResult

	// Make creates a new validator instance with rules.
	Make(data any, rules map[string]string) Validator

	// AddRule adds a custom validation rule.
	AddRule(name string, rule ValidationRule)

	// SetMessages sets custom error messages.
	SetMessages(messages map[string]string)

	// SetAttributeNames sets custom attribute names for error messages.
	SetAttributeNames(names map[string]string)
}

// ValidationResult represents the result of validation.
type ValidationResult interface {
	// Passes returns true if validation passed.
	Passes() bool

	// Fails returns true if validation failed.
	Fails() bool

	// Errors returns all validation errors.
	Errors() ValidationErrors

	// First returns the first error message.
	First() string

	// FirstFor returns the first error message for a field.
	FirstFor(field string) string

	// Get returns all error messages for a field.
	Get(field string) []string

	// All returns all error messages as a flat slice.
	All() []string

	// Messages returns all error messages keyed by field.
	Messages() map[string][]string

	// Validated returns the validated data.
	Validated() map[string]any
}

// ValidationErrors represents a collection of validation errors.
type ValidationErrors interface {
	// Has checks if there are errors for a field.
	Has(field string) bool

	// Get returns errors for a field.
	Get(field string) []string

	// First returns the first error for a field.
	First(field string) string

	// All returns all errors.
	All() map[string][]string

	// Count returns the number of errors.
	Count() int

	// IsEmpty returns true if there are no errors.
	IsEmpty() bool

	// Add adds an error for a field.
	Add(field, message string)

	// ToJSON returns the errors as JSON.
	ToJSON() ([]byte, error)
}

// ValidationRule defines a custom validation rule.
type ValidationRule interface {
	// Passes checks if the validation rule passes.
	Passes(attribute string, value any) bool

	// Message returns the validation error message.
	Message() string
}
