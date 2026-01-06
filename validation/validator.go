// Package validation provides input validation using go-playground/validator.
package validation

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/go-playground/validator/v10"
)

// Validator wraps go-playground/validator with Laravel-like API.
type Validator struct {
	validate       *validator.Validate
	customMessages map[string]string
	attributeNames map[string]string
	mu             sync.RWMutex
}

// New creates a new Validator instance.
func New() *Validator {
	v := validator.New()

	// Use JSON tag names for field names
	v.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name := strings.SplitN(fld.Tag.Get("json"), ",", 2)[0]
		if name == "-" {
			return ""
		}
		return name
	})

	return &Validator{
		validate:       v,
		customMessages: make(map[string]string),
		attributeNames: make(map[string]string),
	}
}

// Validate validates the given struct.
func (v *Validator) Validate(data any) *ValidationResult {
	err := v.validate.Struct(data)
	return v.newResult(err, nil)
}

// ValidateMap validates a map against rules.
func (v *Validator) ValidateMap(data map[string]any, rules map[string]string) *ValidationResult {
	// Convert rules to map[string]any
	rulesAny := make(map[string]any, len(rules))
	for k, val := range rules {
		rulesAny[k] = val
	}

	errs := v.validate.ValidateMap(data, rulesAny)

	if len(errs) == 0 {
		return &ValidationResult{
			valid:     true,
			validated: data,
		}
	}

	errors := NewValidationErrors()
	for field, err := range errs {
		if validationErr, ok := err.(validator.ValidationErrors); ok {
			for _, fe := range validationErr {
				errors.Add(field, v.formatMapError(fe, field))
			}
		} else if e, ok := err.(error); ok {
			errors.Add(field, e.Error())
		} else {
			errors.Add(field, "validation failed")
		}
	}

	return &ValidationResult{
		valid:     false,
		errors:    errors,
		validated: data,
	}
}

// ValidateValue validates a single value.
func (v *Validator) ValidateValue(value any, rules string) error {
	return v.validate.Var(value, rules)
}

// newResult creates a ValidationResult from validator errors.
func (v *Validator) newResult(err error, data map[string]any) *ValidationResult {
	if err == nil {
		return &ValidationResult{
			valid:     true,
			validated: data,
		}
	}

	errors := NewValidationErrors()

	if validationErrors, ok := err.(validator.ValidationErrors); ok {
		for _, fe := range validationErrors {
			field := fe.Field()
			errors.Add(field, v.formatError(fe))
		}
	}

	return &ValidationResult{
		valid:     false,
		errors:    errors,
		validated: data,
	}
}

// formatError formats a validation error message.
func (v *Validator) formatError(fe validator.FieldError) string {
	return v.formatErrorWithField(fe, "")
}

// formatMapError formats a validation error message for a map field.
func (v *Validator) formatMapError(fe validator.FieldError, fieldName string) string {
	return v.formatErrorWithField(fe, fieldName)
}

// formatErrorWithField formats a validation error message with an optional field name override.
func (v *Validator) formatErrorWithField(fe validator.FieldError, fieldNameOverride string) string {
	v.mu.RLock()
	defer v.mu.RUnlock()

	// Determine field name to use for key lookup and display
	lookupField := fe.Field()
	if lookupField == "" && fieldNameOverride != "" {
		lookupField = fieldNameOverride
	}

	// Check for custom message
	key := lookupField + "." + fe.Tag()
	if msg, ok := v.customMessages[key]; ok {
		return v.replaceMessagePlaceholders(msg, fe, fieldNameOverride)
	}

	// Default messages
	return v.defaultMessage(fe, fieldNameOverride)
}

// defaultMessage returns the default error message for a validation tag.
func (v *Validator) defaultMessage(fe validator.FieldError, fieldNameOverride string) string {
	var field string
	if fieldNameOverride != "" {
		field = v.getAttributeName(fieldNameOverride)
	} else {
		field = v.getAttributeName(fe.Field())
	}

	switch fe.Tag() {
	case "required":
		return field + " is required"
	case "email":
		return field + " must be a valid email address"
	case "min":
		return field + " must be at least " + fe.Param() + " characters"
	case "max":
		return field + " must not exceed " + fe.Param() + " characters"
	case "len":
		return field + " must be exactly " + fe.Param() + " characters"
	case "gt":
		return field + " must be greater than " + fe.Param()
	case "gte":
		return field + " must be greater than or equal to " + fe.Param()
	case "lt":
		return field + " must be less than " + fe.Param()
	case "lte":
		return field + " must be less than or equal to " + fe.Param()
	case "eq":
		return field + " must be equal to " + fe.Param()
	case "ne":
		return field + " must not be equal to " + fe.Param()
	case "oneof":
		return field + " must be one of: " + fe.Param()
	case "url":
		return field + " must be a valid URL"
	case "uuid":
		return field + " must be a valid UUID"
	case "alpha":
		return field + " must contain only alphabetic characters"
	case "alphanum":
		return field + " must contain only alphanumeric characters"
	case "numeric":
		return field + " must be numeric"
	case "number":
		return field + " must be a number"
	case "boolean":
		return field + " must be a boolean"
	case "json":
		return field + " must be valid JSON"
	case "datetime":
		return field + " must be a valid datetime"
	case "eqfield":
		return field + " must match " + fe.Param()
	case "nefield":
		return field + " must not match " + fe.Param()
	case "contains":
		return field + " must contain " + fe.Param()
	case "excludes":
		return field + " must not contain " + fe.Param()
	case "startswith":
		return field + " must start with " + fe.Param()
	case "endswith":
		return field + " must end with " + fe.Param()
	case "ip":
		return field + " must be a valid IP address"
	case "ipv4":
		return field + " must be a valid IPv4 address"
	case "ipv6":
		return field + " must be a valid IPv6 address"
	default:
		return field + " failed validation: " + fe.Tag()
	}
}

// getAttributeName returns the display name for a field.
func (v *Validator) getAttributeName(field string) string {
	if name, ok := v.attributeNames[field]; ok {
		return name
	}
	// Convert camelCase/snake_case to Title Case
	return strings.Title(strings.ReplaceAll(strings.ReplaceAll(field, "_", " "), "-", " "))
}

// replaceMessagePlaceholders replaces placeholders in custom messages.
func (v *Validator) replaceMessagePlaceholders(msg string, fe validator.FieldError, fieldNameOverride string) string {
	var field string
	if fieldNameOverride != "" {
		field = v.getAttributeName(fieldNameOverride)
	} else {
		field = v.getAttributeName(fe.Field())
	}

	msg = strings.ReplaceAll(msg, ":attribute", field)

	// Safely convert value to string, handling all types
	var valueStr string
	if val := fe.Value(); val != nil {
		valueStr = fmt.Sprintf("%v", val)
	}
	msg = strings.ReplaceAll(msg, ":value", valueStr)
	msg = strings.ReplaceAll(msg, ":param", fe.Param())
	return msg
}

// SetMessages sets custom validation messages.
func (v *Validator) SetMessages(messages map[string]string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	for k, val := range messages {
		v.customMessages[k] = val
	}
}

// SetAttributeNames sets custom attribute names for error messages.
func (v *Validator) SetAttributeNames(names map[string]string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	for k, val := range names {
		v.attributeNames[k] = val
	}
}

// RegisterValidation registers a custom validation function.
func (v *Validator) RegisterValidation(tag string, fn validator.Func) error {
	return v.validate.RegisterValidation(tag, fn)
}

// RegisterCustomTypeFunc registers a custom type function.
func (v *Validator) RegisterCustomTypeFunc(fn validator.CustomTypeFunc, types ...any) {
	v.validate.RegisterCustomTypeFunc(fn, types...)
}

// ValidationResult holds the result of validation.
type ValidationResult struct {
	valid     bool
	errors    *ValidationErrors
	validated map[string]any
}

// Passes returns true if validation passed.
func (r *ValidationResult) Passes() bool {
	return r.valid
}

// Fails returns true if validation failed.
func (r *ValidationResult) Fails() bool {
	return !r.valid
}

// Errors returns all validation errors.
func (r *ValidationResult) Errors() *ValidationErrors {
	if r.errors == nil {
		return NewValidationErrors()
	}
	return r.errors
}

// First returns the first error message.
func (r *ValidationResult) First() string {
	if r.errors == nil || r.errors.IsEmpty() {
		return ""
	}
	for _, messages := range r.errors.All() {
		if len(messages) > 0 {
			return messages[0]
		}
	}
	return ""
}

// FirstFor returns the first error message for a field.
func (r *ValidationResult) FirstFor(field string) string {
	if r.errors == nil {
		return ""
	}
	return r.errors.First(field)
}

// Get returns all error messages for a field.
func (r *ValidationResult) Get(field string) []string {
	if r.errors == nil {
		return nil
	}
	return r.errors.Get(field)
}

// All returns all error messages as a flat slice.
func (r *ValidationResult) All() []string {
	if r.errors == nil {
		return nil
	}
	var all []string
	for _, messages := range r.errors.All() {
		all = append(all, messages...)
	}
	return all
}

// Messages returns all error messages keyed by field.
func (r *ValidationResult) Messages() map[string][]string {
	if r.errors == nil {
		return nil
	}
	return r.errors.All()
}

// Validated returns the validated data.
func (r *ValidationResult) Validated() map[string]any {
	return r.validated
}

// ValidationErrors holds validation errors.
type ValidationErrors struct {
	errors map[string][]string
	mu     sync.RWMutex
}

// NewValidationErrors creates a new ValidationErrors instance.
func NewValidationErrors() *ValidationErrors {
	return &ValidationErrors{
		errors: make(map[string][]string),
	}
}

// Has checks if there are errors for a field.
func (e *ValidationErrors) Has(field string) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	_, ok := e.errors[field]
	return ok
}

// Get returns errors for a field.
func (e *ValidationErrors) Get(field string) []string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.errors[field]
}

// First returns the first error for a field.
func (e *ValidationErrors) First(field string) string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if messages, ok := e.errors[field]; ok && len(messages) > 0 {
		return messages[0]
	}
	return ""
}

// All returns all errors.
func (e *ValidationErrors) All() map[string][]string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// Return a copy
	result := make(map[string][]string, len(e.errors))
	for k, v := range e.errors {
		result[k] = append([]string{}, v...)
	}
	return result
}

// Count returns the number of errors.
func (e *ValidationErrors) Count() int {
	e.mu.RLock()
	defer e.mu.RUnlock()

	count := 0
	for _, messages := range e.errors {
		count += len(messages)
	}
	return count
}

// IsEmpty returns true if there are no errors.
func (e *ValidationErrors) IsEmpty() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.errors) == 0
}

// Add adds an error for a field.
func (e *ValidationErrors) Add(field, message string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.errors[field] = append(e.errors[field], message)
}

// ToJSON returns the errors as JSON.
func (e *ValidationErrors) ToJSON() ([]byte, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return json.Marshal(e.errors)
}

// Error implements the error interface.
func (e *ValidationErrors) Error() string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var msgs []string
	for field, messages := range e.errors {
		for _, msg := range messages {
			msgs = append(msgs, field+": "+msg)
		}
	}
	return strings.Join(msgs, "; ")
}
