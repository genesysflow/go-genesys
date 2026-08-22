package validation_test

import (
	"testing"

	"github.com/genesysflow/go-genesys/validation"
	"github.com/stretchr/testify/assert"
)

// A rule describes a value. An attribute that was not submitted has no
// value to describe, so everything but the rules about presence itself
// is skipped - otherwise "max=10" reports that a field nobody filled in
// is too long.
func TestValidateMapSkipsRulesForAbsentKeys(t *testing.T) {
	v := validation.New()

	result := v.ValidateMap(map[string]any{}, map[string]string{
		"slug":  "max=140",
		"email": "email",
		"age":   "gte=18",
	})

	assert.True(t, result.Passes(), "absent attributes should not fail rules about their value: %v", result.Messages())
}

// Presence is exactly what a missing attribute does fail.
func TestValidateMapStillRequiresAbsentKeys(t *testing.T) {
	v := validation.New()

	result := v.ValidateMap(map[string]any{}, map[string]string{"slug": "required,max=140"})

	assert.False(t, result.Passes())
	assert.Contains(t, result.Messages()["slug"][0], "required")
}

// A key that is present but empty is still checked: "" is a value.
func TestValidateMapChecksPresentButEmptyValues(t *testing.T) {
	v := validation.New()

	result := v.ValidateMap(map[string]any{"slug": ""}, map[string]string{"slug": "required"})

	assert.False(t, result.Passes())
}

// A rule about presence still applies to an absent key, even a
// conditional one - it is not quietly dropped. Map validation has no
// sibling context, so a conditional presence rule cannot read the field
// it names and fails closed rather than passing silently.
func TestValidateMapKeepsPresenceRulesForAbsentKeys(t *testing.T) {
	v := validation.New()

	result := v.ValidateMap(
		map[string]any{"notify": "yes"},
		map[string]string{"email": "required_with=notify,email"},
	)

	assert.False(t, result.Passes())

	// The failure is about presence, not about the address being
	// malformed: there is no address to malform.
	assert.NotContains(t, result.Messages()["email"][0], "valid email")
}
