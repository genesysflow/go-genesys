package providers

import (
	"testing"

	"github.com/genesysflow/go-genesys/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFilesystemServiceProviderRegister(t *testing.T) {
	cfg := testutil.NewMockConfig(map[string]any{
		"filesystem.default": "local",
		"filesystem.disks": map[string]any{
			"local": map[string]any{
				"driver": "local",
				"root":   "/tmp",
			},
		},
	})
	app := testutil.NewMockApplicationWithConfig(cfg)
	provider := &FilesystemServiceProvider{}

	err := provider.Register(app)
	require.NoError(t, err)

	// Check that filesystem was registered as singleton
	assert.True(t, app.Has("filesystem"))
	assert.True(t, app.Has("filesystem.disk"))
}

func TestFilesystemServiceProviderProvides(t *testing.T) {
	provider := &FilesystemServiceProvider{}
	provides := provider.Provides()

	assert.Contains(t, provides, "filesystem")
	assert.Contains(t, provides, "filesystem.disk")
}
