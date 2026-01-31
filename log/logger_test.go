package log

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/genesysflow/go-genesys/contracts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	logger := New()
	assert.NotNil(t, logger)
	assert.Equal(t, contracts.LogLevelInfo, logger.Level())
}

func TestNewWithWriter(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := New(buf)
	assert.NotNil(t, logger)

	logger.Info("test message")
	assert.Contains(t, buf.String(), "test message")
}

func TestNewJSON(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewJSON(buf)
	assert.NotNil(t, logger)

	logger.Info("json message")

	// Should output valid JSON
	output := buf.String()
	var result map[string]any
	err := json.Unmarshal([]byte(output), &result)
	require.NoError(t, err)
	assert.Equal(t, "json message", result["message"])
}

func TestNewFile(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "test.log")

	logger, err := NewFile(logFile)
	require.NoError(t, err)
	assert.NotNil(t, logger)

	logger.Info("file message")

	// Read the file and verify
	content, err := os.ReadFile(logFile)
	require.NoError(t, err)
	assert.Contains(t, string(content), "file message")
}

func TestNewFileInvalidPath(t *testing.T) {
	_, err := NewFile("/nonexistent/directory/test.log")
	assert.Error(t, err)
}

func TestDebug(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewJSON(buf)
	logger.SetLevel(contracts.LogLevelDebug)

	logger.Debug("debug message")
	assert.Contains(t, buf.String(), "debug message")
	assert.Contains(t, buf.String(), "debug")
}

func TestInfo(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewJSON(buf)

	logger.Info("info message")
	assert.Contains(t, buf.String(), "info message")
	assert.Contains(t, buf.String(), "info")
}

func TestWarn(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewJSON(buf)

	logger.Warn("warning message")
	assert.Contains(t, buf.String(), "warning message")
	assert.Contains(t, buf.String(), "warn")
}

func TestError(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewJSON(buf)

	logger.Error("error message")
	assert.Contains(t, buf.String(), "error message")
	assert.Contains(t, buf.String(), "error")
}

func TestLogWithInlineFields(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewJSON(buf)

	logger.Info("message with fields", "key1", "value1", "key2", 42)

	var result map[string]any
	err := json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)
	assert.Equal(t, "message with fields", result["message"])
	assert.Equal(t, "value1", result["key1"])
	assert.Equal(t, float64(42), result["key2"])
}

func TestWithField(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewJSON(buf)

	newLogger := logger.WithField("user_id", 123)
	newLogger.Info("with field")

	var result map[string]any
	err := json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)
	assert.Equal(t, float64(123), result["user_id"])
}

func TestWithFieldsDoesNotModifyOriginal(t *testing.T) {
	buf1 := &bytes.Buffer{}
	logger := NewJSON(buf1)

	newLogger := logger.WithField("key", "value")

	// Original logger should not have the field
	buf2 := &bytes.Buffer{}
	logger.SetOutput(buf2)
	logger.Info("original")

	assert.NotContains(t, buf2.String(), "key")

	// New logger should have the field
	buf3 := &bytes.Buffer{}
	newLogger.(*Logger).SetOutput(buf3)
	newLogger.(*Logger).Info("new")

	assert.Contains(t, buf3.String(), "key")
}

func TestWithFields(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewJSON(buf)

	newLogger := logger.WithFields(map[string]any{
		"service": "api",
		"version": "1.0.0",
	})
	newLogger.Info("with fields")

	var result map[string]any
	err := json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)
	assert.Equal(t, "api", result["service"])
	assert.Equal(t, "1.0.0", result["version"])
}

func TestWithContext(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewJSON(buf)

	ctx := context.WithValue(context.Background(), "request_id", "req-123")
	newLogger := logger.WithContext(ctx)
	newLogger.Info("with context")

	var result map[string]any
	err := json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)
	assert.Equal(t, "req-123", result["request_id"])
}

func TestWithError(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewJSON(buf)

	newLogger := logger.WithError(assert.AnError)
	newLogger.Info("with error")

	var result map[string]any
	err := json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)
	assert.Contains(t, result["error"], "assert.AnError")
}

func TestSetLevel(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewJSON(buf)

	// Set level to Warn - Debug and Info should not appear
	logger.SetLevel(contracts.LogLevelWarn)

	logger.Debug("debug")
	logger.Info("info")
	logger.Warn("warn")
	logger.Error("error")

	output := buf.String()
	assert.NotContains(t, output, "debug")
	assert.NotContains(t, output, "\"info\"")
	assert.Contains(t, output, "warn")
	assert.Contains(t, output, "error")
}

func TestLevel(t *testing.T) {
	logger := New()
	assert.Equal(t, contracts.LogLevelInfo, logger.Level())

	logger.SetLevel(contracts.LogLevelDebug)
	assert.Equal(t, contracts.LogLevelDebug, logger.Level())

	logger.SetLevel(contracts.LogLevelError)
	assert.Equal(t, contracts.LogLevelError, logger.Level())
}

func TestSetOutput(t *testing.T) {
	buf1 := &bytes.Buffer{}
	buf2 := &bytes.Buffer{}

	logger := NewJSON(buf1)
	logger.Info("to buf1")
	assert.Contains(t, buf1.String(), "to buf1")

	logger.SetOutput(buf2)
	logger.Info("to buf2")
	assert.Contains(t, buf2.String(), "to buf2")
	assert.NotContains(t, buf1.String(), "to buf2")
}

func TestChainedWithMethods(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewJSON(buf)

	logger.
		WithField("key1", "value1").
		WithFields(map[string]any{"key2": "value2"}).
		(*Logger).Info("chained")

	var result map[string]any
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	err := json.Unmarshal([]byte(lines[len(lines)-1]), &result)
	require.NoError(t, err)
	assert.Equal(t, "value1", result["key1"])
	assert.Equal(t, "value2", result["key2"])
}

func TestAllLogLevels(t *testing.T) {
	tests := []struct {
		name  string
		level contracts.LogLevel
	}{
		{"debug", contracts.LogLevelDebug},
		{"info", contracts.LogLevelInfo},
		{"warn", contracts.LogLevelWarn},
		{"error", contracts.LogLevelError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := New()
			logger.SetLevel(tt.level)
			assert.Equal(t, tt.level, logger.Level())
		})
	}
}
