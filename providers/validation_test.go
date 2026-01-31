package providers

import (
	"testing"

	"github.com/genesysflow/go-genesys/testutil"
	"github.com/genesysflow/go-genesys/validation"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidationServiceProviderRegister(t *testing.T) {
	app := testutil.NewMockApplication()
	provider := &ValidationServiceProvider{}

	err := provider.Register(app)
	require.NoError(t, err)

	// Check that validator was registered
	validator := app.GetInstance("validator")
	assert.NotNil(t, validator)
	assert.IsType(t, &validation.Validator{}, validator)
}

func TestValidationServiceProviderRegisterWithCustomMessages(t *testing.T) {
	app := testutil.NewMockApplication()
	provider := &ValidationServiceProvider{
		CustomMessages: map[string]string{
			"required": "The :attribute field is required.",
			"email":    "The :attribute must be a valid email.",
		},
	}

	err := provider.Register(app)
	require.NoError(t, err)

	validator := app.GetInstance("validator")
	assert.NotNil(t, validator)
}

func TestValidationServiceProviderRegisterWithAttributeNames(t *testing.T) {
	app := testutil.NewMockApplication()
	provider := &ValidationServiceProvider{
		AttributeNames: map[string]string{
			"email":      "email address",
			"first_name": "first name",
		},
	}

	err := provider.Register(app)
	require.NoError(t, err)

	validator := app.GetInstance("validator")
	assert.NotNil(t, validator)
}

func TestValidationServiceProviderRegisterWithBothOptions(t *testing.T) {
	app := testutil.NewMockApplication()
	provider := &ValidationServiceProvider{
		CustomMessages: map[string]string{
			"required": "The :attribute field is required.",
		},
		AttributeNames: map[string]string{
			"email": "email address",
		},
	}

	err := provider.Register(app)
	require.NoError(t, err)

	validator := app.GetInstance("validator")
	assert.NotNil(t, validator)
}

func TestValidationServiceProviderBoot(t *testing.T) {
	app := testutil.NewMockApplication()
	provider := &ValidationServiceProvider{}

	err := provider.Register(app)
	require.NoError(t, err)

	err = provider.Boot(app)
	require.NoError(t, err)
}

func TestValidationServiceProviderProvides(t *testing.T) {
	provider := &ValidationServiceProvider{}
	provides := provider.Provides()

	assert.Contains(t, provides, "validator")
}
