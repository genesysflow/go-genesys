package config_test

import (
	"testing"

	"github.com/genesysflow/go-genesys/facades/config"
	"github.com/genesysflow/go-genesys/testutil"
	"github.com/stretchr/testify/assert"
)

func TestFacadeAccessors(t *testing.T) {
	config.SetInstance(testutil.NewMockConfig(map[string]any{
		"app.name":  "MyApp",
		"app.debug": true,
		"app.port":  8080,
	}))
	t.Cleanup(func() { config.SetInstance(nil) })

	assert.Equal(t, "MyApp", config.GetString("app.name"))
	assert.Equal(t, "fallback", config.GetString("app.missing", "fallback"))
	assert.True(t, config.GetBool("app.debug"))
	assert.Equal(t, 8080, config.GetInt("app.port"))
	assert.Equal(t, "MyApp", config.Get("app.name"))
	assert.True(t, config.Has("app.name"))
	assert.False(t, config.Has("app.missing"))
	assert.NotNil(t, config.GetInstance())
}

func TestFacadePanicsWithoutInstance(t *testing.T) {
	config.SetInstance(nil)
	assert.Panics(t, func() { config.GetString("x") })
}
