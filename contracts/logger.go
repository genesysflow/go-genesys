package contracts

import "context"

// LogLevel represents the severity of a log message.
type LogLevel int

const (
	LogLevelDebug LogLevel = iota
	LogLevelInfo
	LogLevelWarn
	LogLevelError
	LogLevelFatal
	LogLevelPanic
)

// Logger defines the interface for logging.
type Logger interface {
	// Debug logs a debug message.
	Debug(msg string, fields ...any)

	// Info logs an info message.
	Info(msg string, fields ...any)

	// Warn logs a warning message.
	Warn(msg string, fields ...any)

	// Error logs an error message.
	Error(msg string, fields ...any)

	// Fatal logs a fatal message and exits.
	Fatal(msg string, fields ...any)

	// Panic logs a panic message and panics.
	Panic(msg string, fields ...any)

	// WithField returns a logger with a field attached.
	WithField(key string, value any) Logger

	// WithFields returns a logger with multiple fields attached.
	WithFields(fields map[string]any) Logger

	// WithContext returns a logger with context attached.
	WithContext(ctx context.Context) Logger

	// WithError returns a logger with an error attached.
	WithError(err error) Logger

	// Level returns the current log level.
	Level() LogLevel

	// SetLevel sets the log level.
	SetLevel(level LogLevel)
}

// LogChannel represents a logging channel/driver.
type LogChannel interface {
	Logger

	// Name returns the channel name.
	Name() string
}

// LogManager manages multiple log channels.
type LogManager interface {
	Logger

	// Channel returns a specific log channel.
	Channel(name string) Logger

	// Stack creates a logger that writes to multiple channels.
	Stack(channels ...string) Logger
}
