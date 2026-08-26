// Package log provides structured logging using zerolog.
// It supports multiple log channels and Laravel-style logging API.
package log

import (
	"context"
	"io"
	"os"
	"sync"
	"time"

	"github.com/genesysflow/go-genesys/contracts"
	"github.com/rs/zerolog"
)

// contextKey is a private type for context keys the logger reads, so
// they cannot collide with keys from other packages.
type contextKey struct{ name string }

// RequestIDKey is the context key WithContext inspects for a request
// id; set it with context.WithValue(ctx, log.RequestIDKey, id). The
// bare string key "request_id" is also honoured for compatibility.
var RequestIDKey = contextKey{"request_id"}

// osExit terminates the process after a fatal record has been written.
// It is a variable so tests can swap in a recorder instead of killing
// the test binary.
var osExit = os.Exit

// fatalWriter is implemented by loggers that can emit a fatal or panic
// record without terminating or unwinding. A stack uses it to fan the
// record out to every member before exiting or panicking exactly once.
type fatalWriter interface {
	writeFatal(msg string, fields ...any)
	writePanic(msg string, fields ...any)
}

// Logger is the default logger implementation using zerolog.
type Logger struct {
	mu     sync.RWMutex
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
		// The zerolog level must match the reported default: zerolog's
		// zero value passes everything, which would leak debug records
		// from a logger whose Level() says Info.
		logger: zerolog.New(writer).Level(zerolog.InfoLevel).With().Timestamp().Logger(),
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
		logger: zerolog.New(writer).Level(zerolog.InfoLevel).With().Timestamp().Logger(),
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

// Fatal logs a fatal message and then terminates the process with exit
// code 1. The record is written first; as with zerolog's own Fatal, the
// process exits even when the configured level filters the record out.
func (l *Logger) Fatal(msg string, fields ...any) {
	l.writeFatal(msg, fields...)
	osExit(1)
}

// Panic logs a panic message and then panics with msg. The record is
// written first; as with zerolog's own Panic, the panic happens even
// when the configured level filters the record out.
func (l *Logger) Panic(msg string, fields ...any) {
	l.writePanic(msg, fields...)
	panic(msg)
}

// writeFatal writes the fatal record without exiting.
func (l *Logger) writeFatal(msg string, fields ...any) {
	l.log(zerolog.FatalLevel, msg, fields...)
}

// writePanic writes the panic record without panicking.
func (l *Logger) writePanic(msg string, fields ...any) {
	l.log(zerolog.PanicLevel, msg, fields...)
}

// log is the internal logging method.
func (l *Logger) log(level zerolog.Level, msg string, fields ...any) {
	l.mu.RLock()
	logger := l.logger
	l.mu.RUnlock()
	event := logger.WithLevel(level)

	// Add stored fields
	for k, v := range l.fields {
		event = event.Interface(k, v)
	}

	// Add context values if present
	if l.ctx != nil {
		reqID := l.ctx.Value(RequestIDKey)
		if reqID == nil {
			reqID = l.ctx.Value("request_id") // legacy string key
		}
		if reqID != nil {
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
	return l.WithFields(map[string]any{key: value})
}

// WithFields returns a logger with multiple fields attached.
func (l *Logger) WithFields(fields map[string]any) contracts.Logger {
	l.mu.RLock()
	defer l.mu.RUnlock()
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
	l.mu.RLock()
	defer l.mu.RUnlock()
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
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.level
}

// SetLevel sets the log level.
func (l *Logger) SetLevel(level contracts.LogLevel) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level
	l.logger = l.logger.Level(toZerologLevel(level))
}

// SetOutput sets the output writer.
func (l *Logger) SetOutput(w io.Writer) {
	l.mu.Lock()
	defer l.mu.Unlock()
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
	mu       sync.RWMutex
	channels map[string]contracts.Logger
	default_ string
}

// defaultLogger returns the current default channel under the read lock.
func (m *LogManager) defaultLogger() contracts.Logger {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.channels[m.default_]
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
	m.mu.RLock()
	defer m.mu.RUnlock()
	if ch, ok := m.channels[name]; ok {
		return ch
	}
	return m.channels[m.default_]
}

// Stack creates a logger that writes every record to all of the named
// channels. Unknown names resolve to the default channel, exactly as
// Channel does, and a name that resolves to a channel already in the
// stack is only added once, so a record is never duplicated. Calling it
// without names, or with names that all collapse onto a single channel,
// returns that channel itself rather than a wrapper.
func (m *LogManager) Stack(channels ...string) contracts.Logger {
	if len(channels) == 0 {
		return m.defaultLogger()
	}

	m.mu.RLock()
	seen := make(map[string]bool, len(channels))
	loggers := make([]contracts.Logger, 0, len(channels))
	for _, name := range channels {
		if _, ok := m.channels[name]; !ok {
			name = m.default_
		}
		if seen[name] {
			continue
		}
		seen[name] = true
		if logger := m.channels[name]; logger != nil {
			loggers = append(loggers, logger)
		}
	}
	m.mu.RUnlock()

	switch len(loggers) {
	case 0:
		return m.defaultLogger()
	case 1:
		return loggers[0]
	default:
		return &stackLogger{loggers: loggers}
	}
}

// AddChannel adds a channel to the manager.
func (m *LogManager) AddChannel(name string, logger contracts.Logger) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.channels[name] = logger
}

// SetDefault sets the default channel.
func (m *LogManager) SetDefault(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.channels[name]; ok {
		m.default_ = name
	}
}

// Debug logs a debug message to the default channel.
func (m *LogManager) Debug(msg string, fields ...any) {
	m.defaultLogger().Debug(msg, fields...)
}

// Info logs an info message to the default channel.
func (m *LogManager) Info(msg string, fields ...any) {
	m.defaultLogger().Info(msg, fields...)
}

// Warn logs a warning message to the default channel.
func (m *LogManager) Warn(msg string, fields ...any) {
	m.defaultLogger().Warn(msg, fields...)
}

// Error logs an error message to the default channel.
func (m *LogManager) Error(msg string, fields ...any) {
	m.defaultLogger().Error(msg, fields...)
}

// Fatal logs a fatal message to the default channel, which writes the
// record and then terminates the process with exit code 1.
func (m *LogManager) Fatal(msg string, fields ...any) {
	m.defaultLogger().Fatal(msg, fields...)
}

// Panic logs a panic message to the default channel, which writes the
// record and then panics with msg.
func (m *LogManager) Panic(msg string, fields ...any) {
	m.defaultLogger().Panic(msg, fields...)
}

// WithField returns a logger with a field attached.
func (m *LogManager) WithField(key string, value any) contracts.Logger {
	return m.defaultLogger().WithField(key, value)
}

// WithFields returns a logger with multiple fields attached.
func (m *LogManager) WithFields(fields map[string]any) contracts.Logger {
	return m.defaultLogger().WithFields(fields)
}

// WithContext returns a logger with context attached.
func (m *LogManager) WithContext(ctx context.Context) contracts.Logger {
	return m.defaultLogger().WithContext(ctx)
}

// WithError returns a logger with an error attached.
func (m *LogManager) WithError(err error) contracts.Logger {
	return m.defaultLogger().WithError(err)
}

// Level returns the current log level.
func (m *LogManager) Level() contracts.LogLevel {
	return m.defaultLogger().Level()
}

// SetLevel sets the log level.
func (m *LogManager) SetLevel(level contracts.LogLevel) {
	m.defaultLogger().SetLevel(level)
}

// stackLogger fans every record out to a fixed set of channels. It is
// what LogManager.Stack returns for two or more distinct channels.
type stackLogger struct {
	loggers []contracts.Logger
}

var _ contracts.Logger = (*stackLogger)(nil)

// Debug logs a debug message to every channel of the stack.
func (s *stackLogger) Debug(msg string, fields ...any) {
	for _, logger := range s.loggers {
		logger.Debug(msg, fields...)
	}
}

// Info logs an info message to every channel of the stack.
func (s *stackLogger) Info(msg string, fields ...any) {
	for _, logger := range s.loggers {
		logger.Info(msg, fields...)
	}
}

// Warn logs a warning message to every channel of the stack.
func (s *stackLogger) Warn(msg string, fields ...any) {
	for _, logger := range s.loggers {
		logger.Warn(msg, fields...)
	}
}

// Error logs an error message to every channel of the stack.
func (s *stackLogger) Error(msg string, fields ...any) {
	for _, logger := range s.loggers {
		logger.Error(msg, fields...)
	}
}

// Fatal writes the fatal record to every channel and then terminates the
// process once. Channels that cannot write a fatal record without
// exiting (foreign contracts.Logger implementations) are asked to Fatal
// directly and may terminate the process before the remaining channels
// are reached.
func (s *stackLogger) Fatal(msg string, fields ...any) {
	for _, logger := range s.loggers {
		if writer, ok := logger.(fatalWriter); ok {
			writer.writeFatal(msg, fields...)
			continue
		}
		logger.Fatal(msg, fields...)
	}
	osExit(1)
}

// Panic writes the panic record to every channel and then panics once,
// with the same caveat as Fatal for foreign implementations.
func (s *stackLogger) Panic(msg string, fields ...any) {
	for _, logger := range s.loggers {
		if writer, ok := logger.(fatalWriter); ok {
			writer.writePanic(msg, fields...)
			continue
		}
		logger.Panic(msg, fields...)
	}
	panic(msg)
}

// WithField returns a stack whose channels all carry the field.
func (s *stackLogger) WithField(key string, value any) contracts.Logger {
	return s.derive(func(logger contracts.Logger) contracts.Logger {
		return logger.WithField(key, value)
	})
}

// WithFields returns a stack whose channels all carry the fields.
func (s *stackLogger) WithFields(fields map[string]any) contracts.Logger {
	return s.derive(func(logger contracts.Logger) contracts.Logger {
		return logger.WithFields(fields)
	})
}

// WithContext returns a stack whose channels all carry the context.
func (s *stackLogger) WithContext(ctx context.Context) contracts.Logger {
	return s.derive(func(logger contracts.Logger) contracts.Logger {
		return logger.WithContext(ctx)
	})
}

// WithError returns a stack whose channels all carry the error.
func (s *stackLogger) WithError(err error) contracts.Logger {
	return s.derive(func(logger contracts.Logger) contracts.Logger {
		return logger.WithError(err)
	})
}

// derive builds a new stack from the per-channel result of fn.
func (s *stackLogger) derive(fn func(contracts.Logger) contracts.Logger) contracts.Logger {
	derived := make([]contracts.Logger, len(s.loggers))
	for i, logger := range s.loggers {
		derived[i] = fn(logger)
	}
	return &stackLogger{loggers: derived}
}

// Level returns the most verbose level among the stacked channels: a
// record is emitted as long as at least one channel accepts it.
func (s *stackLogger) Level() contracts.LogLevel {
	if len(s.loggers) == 0 {
		return contracts.LogLevelInfo
	}
	level := s.loggers[0].Level()
	for _, logger := range s.loggers[1:] {
		if current := logger.Level(); current < level {
			level = current
		}
	}
	return level
}

// SetLevel sets the log level on every channel of the stack.
func (s *stackLogger) SetLevel(level contracts.LogLevel) {
	for _, logger := range s.loggers {
		logger.SetLevel(level)
	}
}
