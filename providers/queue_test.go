package providers

import (
	"testing"

	"github.com/genesysflow/go-genesys/queue"
	"github.com/genesysflow/go-genesys/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQueueServiceProviderRegister(t *testing.T) {
	app := testutil.NewMockApplication()
	provider := &QueueServiceProvider{}

	err := provider.Register(app)
	require.NoError(t, err)

	// Check that queue manager was registered
	queueManager := app.GetInstance("queue")
	assert.NotNil(t, queueManager)
	assert.IsType(t, &queue.Manager{}, queueManager)
}

func TestQueueServiceProviderBoot(t *testing.T) {
	app := testutil.NewMockApplication()
	provider := &QueueServiceProvider{}

	err := provider.Register(app)
	require.NoError(t, err)

	err = provider.Boot(app)
	require.NoError(t, err)
}

func TestQueueServiceProviderProvides(t *testing.T) {
	provider := &QueueServiceProvider{}
	provides := provider.Provides()

	assert.Contains(t, provides, "queue")
}
