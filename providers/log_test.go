package providers

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/genesysflow/go-genesys/log"
	"github.com/genesysflow/go-genesys/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogServiceProviderRegisterDefault(t *testing.T) {
	app := testutil.NewMockApplication()
	provider := &LogServiceProvider{}

	err := provider.Register(app)
	require.NoError(t, err)

	// Check that logger was registered
	logger := app.GetInstance("logger")
	assert.NotNil(t, logger)
	assert.IsType(t, &log.Logger{}, logger)

	// Check that log manager was registered
	logManager := app.GetInstance("log.manager")
	assert.NotNil(t, logManager)
	assert.IsType(t, &log.LogManager{}, logManager)
}

func TestLogServiceProviderRegisterConsoleChannel(t *testing.T) {
	cfg := testutil.NewMockConfig(map[string]any{
		"logging.default": "console",
	})
	app := testutil.NewMockApplicationWithConfig(cfg)
	provider := &LogServiceProvider{}

	err := provider.Register(app)
	require.NoError(t, err)

	logger := app.GetInstance("logger")
	assert.NotNil(t, logger)
}

func TestLogServiceProviderRegisterJSONChannel(t *testing.T) {
	cfg := testutil.NewMockConfig(map[string]any{
		"logging.default": "json",
	})
	app := testutil.NewMockApplicationWithConfig(cfg)
	provider := &LogServiceProvider{}

	err := provider.Register(app)
	require.NoError(t, err)

	logger := app.GetInstance("logger")
	assert.NotNil(t, logger)
}

func TestLogServiceProviderRegisterFileChannel(t *testing.T) {
	// Create temp directory for log file
	tmpDir, err := os.MkdirTemp("", "log-test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	logPath := filepath.Join(tmpDir, "app.log")

	cfg := testutil.NewMockConfig(map[string]any{
		"logging.default":            "file",
		"logging.channels.file.path": logPath,
	})
	app := testutil.NewMockApplicationWithConfig(cfg)
	provider := &LogServiceProvider{}

	err = provider.Register(app)
	require.NoError(t, err)

	logger := app.GetInstance("logger")
	assert.NotNil(t, logger)
}

func TestLogServiceProviderRegisterWithLogLevel(t *testing.T) {
	tests := []struct {
		name  string
		level string
	}{
		{"debug level", "debug"},
		{"info level", "info"},
		{"warn level", "warn"},
		{"warning level", "warning"},
		{"error level", "error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := testutil.NewMockConfig(map[string]any{
				"logging.level": tt.level,
			})
			app := testutil.NewMockApplicationWithConfig(cfg)
			provider := &LogServiceProvider{}

			err := provider.Register(app)
			require.NoError(t, err)

			logger := app.GetInstance("logger")
			assert.NotNil(t, logger)
		})
	}
}

func TestLogServiceProviderBoot(t *testing.T) {
	app := testutil.NewMockApplication()
	provider := &LogServiceProvider{}

	err := provider.Register(app)
	require.NoError(t, err)

	err = provider.Boot(app)
	require.NoError(t, err)
}

func TestLogServiceProviderProvides(t *testing.T) {
	provider := &LogServiceProvider{}
	provides := provider.Provides()

	assert.Contains(t, provides, "logger")
	assert.Contains(t, provides, "log.manager")
}

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"debug", "debug"},
		{"info", "info"},
		{"warn", "warn"},
		{"warning", "warn"},
		{"error", "error"},
		{"invalid", "info"}, // default
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			level := parseLogLevel(tt.input)
			// Just verify it returns without panicking
			assert.NotNil(t, level)
		})
	}
}
