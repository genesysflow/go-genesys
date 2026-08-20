package config_test

import (
	"testing"

	"github.com/genesysflow/go-genesys/config"
	"github.com/genesysflow/go-genesys/testutil"
	"github.com/stretchr/testify/assert"
)

func TestValidate(t *testing.T) {
	cfg := testutil.NewMockConfig(map[string]any{
		"app.key":          "base64:abc",
		"database.default": "pgsql",
		"mail.from":        "   ", // whitespace counts as missing
	})

	assert.NoError(t, config.Validate(cfg, "app.key", "database.default"))

	err := config.Validate(cfg, "app.key", "app.url", "mail.from", "queue.default")
	assert.ErrorContains(t, err, "app.url")
	assert.ErrorContains(t, err, "mail.from")
	assert.ErrorContains(t, err, "queue.default")
	assert.NotContains(t, err.Error(), "app.key")

	assert.Panics(t, func() { config.MustValidate(cfg, "nope.key") })
	assert.NotPanics(t, func() { config.MustValidate(cfg, "app.key") })
}
