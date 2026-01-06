package validation

import (
	"testing"

	"github.com/go-playground/validator/v10"
	"github.com/stretchr/testify/assert"
)

type User struct {
	Name  string `json:"name" validate:"required"`
	Email string `json:"email" validate:"required,email"`
	Age   int    `json:"age" validate:"gte=0,lte=130"`
}

func TestNew(t *testing.T) {
	v := New()
	assert.NotNil(t, v)
}

func TestValidate_Struct(t *testing.T) {
	v := New()

	// Valid struct
	user := User{Name: "John", Email: "john@example.com", Age: 30}
	result := v.Validate(user)
	assert.True(t, result.Passes())
	assert.False(t, result.Fails())
	assert.Empty(t, result.Errors().All())

	// Invalid struct
	invalidUser := User{Name: "", Email: "invalid", Age: -5}
	result = v.Validate(invalidUser)
	assert.False(t, result.Passes())
	errors := result.Errors()

	assert.Equal(t, "Name is required", errors.First("name"))
	assert.Equal(t, "Email must be a valid email address", errors.First("email"))
	assert.Equal(t, "Age must be greater than or equal to 0", errors.First("age"))
}

func TestValidateMap(t *testing.T) {
	v := New()

	rules := map[string]string{
		"name":  "required",
		"email": "required,email",
		"age":   "gte=0,lte=130",
	}

	// Valid map
	data := map[string]any{
		"name":  "John",
		"email": "john@example.com",
		"age":   30,
	}
	result := v.ValidateMap(data, rules)
	assert.True(t, result.Passes())

	// Invalid map
	invalidData := map[string]any{
		"name":  "",
		"email": "invalid",
		"age":   -5,
	}
	result = v.ValidateMap(invalidData, rules)
	assert.False(t, result.Passes())
	errors := result.Errors()

	t.Logf("Errors: %+v", errors.All())

	assert.Equal(t, "Name is required", errors.First("name"))
	// Note: go-playground/validator might capitalize map keys differently or treat them as fields.
	// In our implementation we use Title Case for "field is required" part in defaultMessage.
	// If the field name coming from validator error matches the map key, then getAttributeName should handle it.
}

func TestValidateValue(t *testing.T) {
	v := New()

	assert.NoError(t, v.ValidateValue("john@example.com", "required,email"))
	assert.Error(t, v.ValidateValue("invalid", "email"))
}

func TestCustomMessages(t *testing.T) {
	v := New()
	v.SetMessages(map[string]string{
		"name.required": "Please provide a name",
	})

	user := User{Name: ""}
	result := v.Validate(user)

	assert.Equal(t, "Please provide a name", result.FirstFor("name"))
}

func TestAttributeNames(t *testing.T) {
	v := New()
	v.SetAttributeNames(map[string]string{
		"age": "User Age",
	})

	user := User{Age: -1}
	result := v.Validate(user)

	// age has gte=0 tag
	assert.Contains(t, result.FirstFor("age"), "User Age must be greater than or equal to 0")
}

func TestRegisterValidation(t *testing.T) {
	v := New()

	// Custom validation that checks if string is "cool"
	v.RegisterValidation("cool", func(fl validator.FieldLevel) bool {
		return fl.Field().String() == "cool"
	})

	type CoolStruct struct {
		Vibe string `validate:"cool"`
	}

	result := v.Validate(CoolStruct{Vibe: "boring"})
	assert.False(t, result.Passes())
	// Default message likely "Vibe failed validation: cool"
	assert.Contains(t, result.FirstFor("Vibe"), "failed validation: cool")

	result = v.Validate(CoolStruct{Vibe: "cool"})
	assert.True(t, result.Passes())
}

func TestValidationResult_Accessors(t *testing.T) {
	v := New()
	user := User{Name: ""}
	result := v.Validate(user)

	assert.False(t, result.Passes())
	assert.True(t, result.Fails())
	assert.NotNil(t, result.Errors())
	assert.Equal(t, 2, result.Errors().Count())
	assert.NotEmpty(t, result.All())

	// Check JSON serialization
	jsonBytes, err := result.Errors().ToJSON()
	assert.NoError(t, err)
	assert.Contains(t, string(jsonBytes), "Name is required")
}
