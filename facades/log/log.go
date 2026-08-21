// Package log provides a static facade for the application logger.
package log

import (
	"sync"

	"github.com/genesysflow/go-genesys/contracts"
)

var (
	instance contracts.Logger
	mu       sync.RWMutex
)

// SetInstance sets the logger instance.
// This is called during application bootstrap.
func SetInstance(logger contracts.Logger) {
	mu.Lock()
	defer mu.Unlock()
	instance = logger
}

// GetInstance returns the logger instance.
func GetInstance() contracts.Logger {
	mu.RLock()
	defer mu.RUnlock()
	return instance
}

func logger() contracts.Logger {
	mu.RLock()
	defer mu.RUnlock()
	if instance == nil {
		panic("log: facade not initialised - register the LogServiceProvider")
	}
	return instance
}

// Debug logs a debug message.
func Debug(msg string, fields ...any) { logger().Debug(msg, fields...) }

// Info logs an info message.
func Info(msg string, fields ...any) { logger().Info(msg, fields...) }

// Warn logs a warning message.
func Warn(msg string, fields ...any) { logger().Warn(msg, fields...) }

// Error logs an error message.
func Error(msg string, fields ...any) { logger().Error(msg, fields...) }

// WithFields returns a logger with the given fields attached.
func WithFields(fields map[string]any) contracts.Logger {
	return logger().WithFields(fields)
}
