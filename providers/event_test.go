package providers

import (
	"testing"

	"github.com/genesysflow/go-genesys/events"
	"github.com/genesysflow/go-genesys/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEventServiceProviderRegister(t *testing.T) {
	app := testutil.NewMockApplication()
	provider := &EventServiceProvider{}

	err := provider.Register(app)
	require.NoError(t, err)

	// Check that event dispatcher was registered
	dispatcher := app.GetInstance("events")
	assert.NotNil(t, dispatcher)
	assert.IsType(t, &events.Dispatcher{}, dispatcher)
}

func TestEventServiceProviderBoot(t *testing.T) {
	app := testutil.NewMockApplication()
	provider := &EventServiceProvider{}

	err := provider.Register(app)
	require.NoError(t, err)

	err = provider.Boot(app)
	require.NoError(t, err)
}

func TestEventServiceProviderProvides(t *testing.T) {
	provider := &EventServiceProvider{}
	provides := provider.Provides()

	assert.Contains(t, provides, "events")
}
