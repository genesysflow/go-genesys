package validation_test

import (
	"testing"

	"github.com/genesysflow/go-genesys/lang"
	"github.com/genesysflow/go-genesys/validation"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type signupForm struct {
	Email string `validate:"required,email" json:"email"`
	Name  string `validate:"required" json:"name"`
}

func TestTranslatedValidationMessages(t *testing.T) {
	translator := lang.New(lang.Config{Path: t.TempDir(), Locale: "de"})
	translator.AddLine("de", "validation.required", ":attribute ist erforderlich")
	translator.AddLine("de", "validation.email", ":attribute muss eine gültige E-Mail sein")
	translator.AddLine("de", "validation.attributes.email", "E-Mail-Adresse")

	v := validation.New()
	v.SetTranslator(translator)

	result := v.Validate(&signupForm{})
	require.True(t, result.Fails())

	messages := result.All()
	assert.Contains(t, messages, "E-Mail-Adresse ist erforderlich",
		"tag message translated and attribute name localized")
	assert.Contains(t, messages, "Name ist erforderlich")
}

func TestPerFieldTranslationBeatsTagTranslation(t *testing.T) {
	translator := lang.New(lang.Config{Path: t.TempDir(), Locale: "en"})
	translator.AddLine("en", "validation.required", ":attribute is required")
	translator.AddLine("en", "validation.email.required", "We need your email address!")

	v := validation.New()
	v.SetTranslator(translator)

	result := v.Validate(&signupForm{})
	require.True(t, result.Fails())
	assert.Contains(t, result.All(), "We need your email address!")
	assert.Contains(t, result.All(), "Name is required")
}

func TestLocaleViewsAsValidatorTranslator(t *testing.T) {
	translator := lang.New(lang.Config{Path: t.TempDir(), Locale: "en"})
	translator.AddLine("fr", "validation.required", ":attribute est obligatoire")

	v := validation.New()
	v.SetTranslator(translator.In("fr")) // per-request pinned locale

	result := v.Validate(&signupForm{})
	require.True(t, result.Fails())
	assert.Contains(t, result.All(), "Name est obligatoire")
}

func TestDefaultsWithoutTranslator(t *testing.T) {
	v := validation.New()
	result := v.Validate(&signupForm{})
	require.True(t, result.Fails())
	assert.Contains(t, result.All(), "Name is required")
}
