// Package log provides structured logging using zerolog.
// It supports multiple log channels and Laravel-style logging API.
package log

import (
	"context"
	"io"
	"os"
	"time"

	"github.com/genesysflow/go-genesys/contracts"
	"github.com/rs/zerolog"
)

// Logger is the default logger implementation using zerolog.
type Logger struct {
	logger zerolog.Logger
	level  contracts.LogLevel
	fields map[string]any
	ctx    context.Context
}

// New creates a new Logger instance.
func New(writers ...io.Writer) *Logger {
	var writer io.Writer
	if len(writers) > 0 {
		writer = writers[0]
	} else {
		// Default to pretty console output
		writer = zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: time.RFC3339,
		}
	}

	return &Logger{
		logger: zerolog.New(writer).With().Timestamp().Logger(),
		level:  contracts.LogLevelInfo,
		fields: make(map[string]any),
	}
}

// NewJSON creates a new Logger with JSON output.
func NewJSON(writers ...io.Writer) *Logger {
	var writer io.Writer
	if len(writers) > 0 {
		writer = writers[0]
	} else {
		writer = os.Stdout
	}

	return &Logger{
		logger: zerolog.New(writer).With().Timestamp().Logger(),
		level:  contracts.LogLevelInfo,
		fields: make(map[string]any),
	}
}

// NewFile creates a new Logger that writes to a file.
func NewFile(path string) (*Logger, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return nil, err
	}
	return NewJSON(file), nil
}

// Debug logs a debug message.
func (l *Logger) Debug(msg string, fields ...any) {
	l.log(zerolog.DebugLevel, msg, fields...)
}

// Info logs an info message.
func (l *Logger) Info(msg string, fields ...any) {
	l.log(zerolog.InfoLevel, msg, fields...)
}

// Warn logs a warning message.
func (l *Logger) Warn(msg string, fields ...any) {
	l.log(zerolog.WarnLevel, msg, fields...)
}

// Error logs an error message.
func (l *Logger) Error(msg string, fields ...any) {
	l.log(zerolog.ErrorLevel, msg, fields...)
}

// Fatal logs a fatal message and exits.
func (l *Logger) Fatal(msg string, fields ...any) {
	l.log(zerolog.FatalLevel, msg, fields...)
}

// Panic logs a panic message and panics.
func (l *Logger) Panic(msg string, fields ...any) {
	l.log(zerolog.PanicLevel, msg, fields...)
}

// log is the internal logging method.
func (l *Logger) log(level zerolog.Level, msg string, fields ...any) {
	event := l.logger.WithLevel(level)

	// Add stored fields
	for k, v := range l.fields {
		event = event.Interface(k, v)
	}

	// Add context values if present
	if l.ctx != nil {
		if reqID := l.ctx.Value("request_id"); reqID != nil {
			event = event.Interface("request_id", reqID)
		}
	}

	// Add inline fields (key-value pairs)
	for i := 0; i < len(fields)-1; i += 2 {
		if key, ok := fields[i].(string); ok {
			event = event.Interface(key, fields[i+1])
		}
	}

	event.Msg(msg)
}

// WithField returns a logger with a field attached.
func (l *Logger) WithField(key string, value any) contracts.Logger {
	newFields := make(map[string]any, len(l.fields)+1)
	for k, v := range l.fields {
		newFields[k] = v
	}
	newFields[key] = value

	return &Logger{
		logger: l.logger,
		level:  l.level,
		fields: newFields,
		ctx:    l.ctx,
	}
}

// WithFields returns a logger with multiple fields attached.
func (l *Logger) WithFields(fields map[string]any) contracts.Logger {
	newFields := make(map[string]any, len(l.fields)+len(fields))
	for k, v := range l.fields {
		newFields[k] = v
	}
	for k, v := range fields {
		newFields[k] = v
	}

	return &Logger{
		logger: l.logger,
		level:  l.level,
		fields: newFields,
		ctx:    l.ctx,
	}
}

// WithContext returns a logger with context attached.
func (l *Logger) WithContext(ctx context.Context) contracts.Logger {
	return &Logger{
		logger: l.logger,
		level:  l.level,
		fields: l.fields,
		ctx:    ctx,
	}
}

// WithError returns a logger with an error attached.
func (l *Logger) WithError(err error) contracts.Logger {
	return l.WithField("error", err.Error())
}

// Level returns the current log level.
func (l *Logger) Level() contracts.LogLevel {
	return l.level
}

// SetLevel sets the log level.
func (l *Logger) SetLevel(level contracts.LogLevel) {
	l.level = level
	l.logger = l.logger.Level(toZerologLevel(level))
}

// SetOutput sets the output writer.
func (l *Logger) SetOutput(w io.Writer) {
	l.logger = l.logger.Output(w)
}

// toZerologLevel converts a contracts.LogLevel to zerolog.Level.
func toZerologLevel(level contracts.LogLevel) zerolog.Level {
	switch level {
	case contracts.LogLevelDebug:
		return zerolog.DebugLevel
	case contracts.LogLevelInfo:
		return zerolog.InfoLevel
	case contracts.LogLevelWarn:
		return zerolog.WarnLevel
	case contracts.LogLevelError:
		return zerolog.ErrorLevel
	case contracts.LogLevelFatal:
		return zerolog.FatalLevel
	case contracts.LogLevelPanic:
		return zerolog.PanicLevel
	default:
		return zerolog.InfoLevel
	}
}

// LogManager manages multiple log channels.
type LogManager struct {
	channels map[string]contracts.Logger
	default_ string
}

// NewManager creates a new LogManager.
func NewManager() *LogManager {
	m := &LogManager{
		channels: make(map[string]contracts.Logger),
		default_: "default",
	}

	// Register default channel
	m.channels["default"] = New()

	return m
}

// Channel returns a specific log channel.
func (m *LogManager) Channel(name string) contracts.Logger {
	if ch, ok := m.channels[name]; ok {
		return ch
	}
	return m.channels[m.default_]
}

// Stack creates a logger that writes to multiple channels.
func (m *LogManager) Stack(channels ...string) contracts.Logger {
	// For now, return the first channel. A proper implementation would
	// create a multi-writer logger.
	if len(channels) > 0 {
		return m.Channel(channels[0])
	}
	return m.channels[m.default_]
}

// AddChannel adds a channel to the manager.
func (m *LogManager) AddChannel(name string, logger contracts.Logger) {
	m.channels[name] = logger
}

// SetDefault sets the default channel.
func (m *LogManager) SetDefault(name string) {
	if _, ok := m.channels[name]; ok {
		m.default_ = name
	}
}

// Debug logs a debug message to the default channel.
func (m *LogManager) Debug(msg string, fields ...any) {
	m.channels[m.default_].Debug(msg, fields...)
}

// Info logs an info message to the default channel.
func (m *LogManager) Info(msg string, fields ...any) {
	m.channels[m.default_].Info(msg, fields...)
}

// Warn logs a warning message to the default channel.
func (m *LogManager) Warn(msg string, fields ...any) {
	m.channels[m.default_].Warn(msg, fields...)
}

// Error logs an error message to the default channel.
func (m *LogManager) Error(msg string, fields ...any) {
	m.channels[m.default_].Error(msg, fields...)
}

// Fatal logs a fatal message to the default channel.
func (m *LogManager) Fatal(msg string, fields ...any) {
	m.channels[m.default_].Fatal(msg, fields...)
}

// Panic logs a panic message to the default channel.
func (m *LogManager) Panic(msg string, fields ...any) {
	m.channels[m.default_].Panic(msg, fields...)
}

// WithField returns a logger with a field attached.
func (m *LogManager) WithField(key string, value any) contracts.Logger {
	return m.channels[m.default_].WithField(key, value)
}

// WithFields returns a logger with multiple fields attached.
func (m *LogManager) WithFields(fields map[string]any) contracts.Logger {
	return m.channels[m.default_].WithFields(fields)
}

// WithContext returns a logger with context attached.
func (m *LogManager) WithContext(ctx context.Context) contracts.Logger {
	return m.channels[m.default_].WithContext(ctx)
}

// WithError returns a logger with an error attached.
func (m *LogManager) WithError(err error) contracts.Logger {
	return m.channels[m.default_].WithError(err)
}

// Level returns the current log level.
func (m *LogManager) Level() contracts.LogLevel {
	return m.channels[m.default_].Level()
}

// SetLevel sets the log level.
func (m *LogManager) SetLevel(level contracts.LogLevel) {
	m.channels[m.default_].SetLevel(level)
}
