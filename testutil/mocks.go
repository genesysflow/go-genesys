package testutil

import (
	"context"
	"reflect"
	"sync"

	"github.com/genesysflow/go-genesys/contracts"
	"github.com/stretchr/testify/mock"
)

// MockConfig is a mock implementation of contracts.Config.
type MockConfig struct {
	mock.Mock
	data map[string]any
	mu   sync.RWMutex
}

// NewMockConfig creates a new MockConfig with the given data.
func NewMockConfig(data map[string]any) *MockConfig {
	if data == nil {
		data = make(map[string]any)
	}
	return &MockConfig{data: data}
}

// Get returns a value from the mock config.
func (m *MockConfig) Get(key string) any {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.data == nil {
		return nil
	}
	return m.data[key]
}

// GetString returns a string value from the mock config.
func (m *MockConfig) GetString(key string) string {
	v := m.Get(key)
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// GetInt returns an int value from the mock config.
func (m *MockConfig) GetInt(key string) int {
	v := m.Get(key)
	if v == nil {
		return 0
	}
	if i, ok := v.(int); ok {
		return i
	}
	return 0
}

// GetInt64 returns an int64 value from the mock config.
func (m *MockConfig) GetInt64(key string) int64 {
	v := m.Get(key)
	if v == nil {
		return 0
	}
	if i, ok := v.(int64); ok {
		return i
	}
	return 0
}

// GetBool returns a bool value from the mock config.
func (m *MockConfig) GetBool(key string) bool {
	v := m.Get(key)
	if v == nil {
		return false
	}
	if b, ok := v.(bool); ok {
		return b
	}
	return false
}

// GetFloat returns a float64 value from the mock config.
func (m *MockConfig) GetFloat(key string) float64 {
	v := m.Get(key)
	if v == nil {
		return 0
	}
	if f, ok := v.(float64); ok {
		return f
	}
	return 0
}

// GetSlice returns a slice value from the mock config.
func (m *MockConfig) GetSlice(key string) []any {
	v := m.Get(key)
	if v == nil {
		return nil
	}
	if s, ok := v.([]any); ok {
		return s
	}
	return nil
}

// GetMap returns a map value from the mock config.
func (m *MockConfig) GetMap(key string) map[string]any {
	v := m.Get(key)
	if v == nil {
		return nil
	}
	if mp, ok := v.(map[string]any); ok {
		return mp
	}
	return nil
}

// GetStringSlice returns a string slice from the mock config.
func (m *MockConfig) GetStringSlice(key string) []string {
	v := m.Get(key)
	if v == nil {
		return nil
	}
	if s, ok := v.([]string); ok {
		return s
	}
	return nil
}

// GetStringMap returns a string map from the mock config.
func (m *MockConfig) GetStringMap(key string) map[string]string {
	v := m.Get(key)
	if v == nil {
		return nil
	}
	if mp, ok := v.(map[string]string); ok {
		return mp
	}
	return nil
}

// Set sets a value in the mock config.
func (m *MockConfig) Set(key string, value any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.data == nil {
		m.data = make(map[string]any)
	}
	m.data[key] = value
}

// Has checks if a key exists in the mock config.
func (m *MockConfig) Has(key string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.data == nil {
		return false
	}
	_, ok := m.data[key]
	return ok
}

// All returns all data in the mock config.
func (m *MockConfig) All() map[string]any {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.data
}

// Merge merges data into the mock config.
func (m *MockConfig) Merge(data map[string]any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.data == nil {
		m.data = make(map[string]any)
	}
	for k, v := range data {
		m.data[k] = v
	}
}

// Load loads configuration (no-op for mock).
func (m *MockConfig) Load(path string) error {
	return nil
}

// MockProvider is a mock implementation of contracts.ServiceProvider.
type MockProvider struct {
	mock.Mock
}

// Register registers services.
func (m *MockProvider) Register(app contracts.Application) error {
	args := m.Called(app)
	return args.Error(0)
}

// Boot boots services.
func (m *MockProvider) Boot(app contracts.Application) error {
	args := m.Called(app)
	return args.Error(0)
}

// Provides returns the list of services this provider provides.
func (m *MockProvider) Provides() []string {
	args := m.Called()
	return args.Get(0).([]string)
}

// MockLogger is a mock implementation of contracts.Logger.
type MockLogger struct {
	mock.Mock
	Messages []string
	level    contracts.LogLevel
}

func (m *MockLogger) Debug(msg string, fields ...any) {
	m.Messages = append(m.Messages, "DEBUG: "+msg)
}

func (m *MockLogger) Info(msg string, fields ...any) {
	m.Messages = append(m.Messages, "INFO: "+msg)
}

func (m *MockLogger) Warn(msg string, fields ...any) {
	m.Messages = append(m.Messages, "WARN: "+msg)
}

func (m *MockLogger) Error(msg string, fields ...any) {
	m.Messages = append(m.Messages, "ERROR: "+msg)
}

func (m *MockLogger) Fatal(msg string, fields ...any) {
	m.Messages = append(m.Messages, "FATAL: "+msg)
}

func (m *MockLogger) Panic(msg string, fields ...any) {
	m.Messages = append(m.Messages, "PANIC: "+msg)
}

func (m *MockLogger) WithField(key string, value any) contracts.Logger {
	return m
}

func (m *MockLogger) WithFields(fields map[string]any) contracts.Logger {
	return m
}

func (m *MockLogger) WithError(err error) contracts.Logger {
	return m
}

func (m *MockLogger) WithContext(ctx context.Context) contracts.Logger {
	return m
}

func (m *MockLogger) Level() contracts.LogLevel {
	return m.level
}

func (m *MockLogger) SetLevel(level contracts.LogLevel) {
	m.level = level
}

// MockApplication is a mock implementation of contracts.Application.
type MockApplication struct {
	mock.Mock
	config    contracts.Config
	logger    contracts.Logger
	bindings  map[string]any
	instances map[string]any
	mu        sync.RWMutex
	basePath  string
	booted    bool
}

// NewMockApplication creates a new MockApplication.
func NewMockApplication() *MockApplication {
	return &MockApplication{
		config:    NewMockConfig(nil),
		logger:    &MockLogger{},
		bindings:  make(map[string]any),
		instances: make(map[string]any),
		basePath:  "/tmp/test-app",
	}
}

// NewMockApplicationWithConfig creates a MockApplication with the given config.
func NewMockApplicationWithConfig(cfg contracts.Config) *MockApplication {
	app := NewMockApplication()
	app.config = cfg
	return app
}

// Version returns the framework version.
func (m *MockApplication) Version() string {
	return "1.0.0-test"
}

// BasePath returns the base path.
func (m *MockApplication) BasePath() string {
	return m.basePath
}

// SetBasePath sets the base path.
func (m *MockApplication) SetBasePath(path string) contracts.Application {
	m.basePath = path
	return m
}

// ConfigPath returns the config path.
func (m *MockApplication) ConfigPath() string {
	return m.basePath + "/config"
}

// StoragePath returns the storage path.
func (m *MockApplication) StoragePath() string {
	return m.basePath + "/storage"
}

// Environment returns the current environment.
func (m *MockApplication) Environment() string {
	return "testing"
}

// IsEnvironment checks if running in given environment(s).
func (m *MockApplication) IsEnvironment(envs ...string) bool {
	for _, env := range envs {
		if env == "testing" {
			return true
		}
	}
	return false
}

// IsProduction returns false for testing.
func (m *MockApplication) IsProduction() bool {
	return false
}

// IsLocal returns true for testing.
func (m *MockApplication) IsLocal() bool {
	return true
}

// IsDebug returns true for testing.
func (m *MockApplication) IsDebug() bool {
	return true
}

// Register registers a service provider.
func (m *MockApplication) Register(provider contracts.ServiceProvider) error {
	return provider.Register(m)
}

// Boot boots the application.
func (m *MockApplication) Boot() error {
	m.booted = true
	return nil
}

// IsBooted returns if the application has been booted.
func (m *MockApplication) IsBooted() bool {
	return m.booted
}

// Booting registers a callback to run before booting.
func (m *MockApplication) Booting(callback func(contracts.Application)) {
	// no-op for mock
}

// Booted registers a callback to run after booting.
func (m *MockApplication) Booted(callback func(contracts.Application)) {
	// no-op for mock
}

// Terminating registers a termination callback.
func (m *MockApplication) Terminating(callback func(contracts.Application)) {
	// no-op for mock
}

// Terminate terminates the application.
func (m *MockApplication) Terminate() error {
	return nil
}

// TerminateWithContext terminates with context.
func (m *MockApplication) TerminateWithContext(ctx context.Context) error {
	return nil
}

// GetConfig returns the config.
func (m *MockApplication) GetConfig() contracts.Config {
	return m.config
}

// GetLogger returns the logger.
func (m *MockApplication) GetLogger() contracts.Logger {
	return m.logger
}

// Bind registers a factory function.
func (m *MockApplication) Bind(name string, factory any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.bindings[name] = factory
	return nil
}

// Singleton registers a singleton factory.
func (m *MockApplication) Singleton(name string, factory any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.bindings[name] = factory
	return nil
}

// BindType registers a factory inferring the name from return type.
func (m *MockApplication) BindType(factory any) error {
	return nil
}

// SingletonType registers a singleton factory inferring name from return type.
func (m *MockApplication) SingletonType(factory any) error {
	return nil
}

// Instance registers an already-created instance.
func (m *MockApplication) Instance(name string, instance any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.instances[name] = instance
	return nil
}

// InstanceType registers an instance inferring name from its type.
func (m *MockApplication) InstanceType(instance any) error {
	t := reflect.TypeOf(instance)
	name := getTypeName(t)
	return m.Instance(name, instance)
}

// getTypeName returns the type name in the same format as container.GetTypeName.
func getTypeName(t reflect.Type) string {
	if t.Kind() == reflect.Ptr {
		elem := t.Elem()
		if elem.PkgPath() != "" {
			return "*" + elem.PkgPath() + "." + elem.Name()
		}
		return t.String()
	}
	if t.PkgPath() != "" {
		return t.PkgPath() + "." + t.Name()
	}
	return t.String()
}

// BindValue is an alias for Instance.
func (m *MockApplication) BindValue(name string, value any) error {
	return m.Instance(name, value)
}

// Make resolves a service by name.
func (m *MockApplication) Make(name string) (any, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if instance, ok := m.instances[name]; ok {
		return instance, nil
	}
	if binding, ok := m.bindings[name]; ok {
		// If it's a factory function, call it
		if factory, ok := binding.(func(contracts.Application) (any, error)); ok {
			return factory(m)
		}
		return binding, nil
	}
	return nil, nil
}

// MustMake resolves a service by name, panicking on error.
func (m *MockApplication) MustMake(name string) any {
	result, err := m.Make(name)
	if err != nil {
		panic(err)
	}
	return result
}

// Has checks if a service is registered.
func (m *MockApplication) Has(name string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, hasInstance := m.instances[name]
	_, hasBinding := m.bindings[name]
	return hasInstance || hasBinding
}

// Shutdown gracefully shuts down services.
func (m *MockApplication) Shutdown() error {
	return nil
}

// ShutdownWithContext shuts down services with context.
func (m *MockApplication) ShutdownWithContext(ctx context.Context) error {
	return nil
}

// GetInstance retrieves an instance directly (test helper).
func (m *MockApplication) GetInstance(name string) any {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.instances[name]
}

// SetConfig sets the config (test helper).
func (m *MockApplication) SetConfig(cfg contracts.Config) {
	m.config = cfg
}

// SetLogger sets the logger (test helper).
func (m *MockApplication) SetLogger(logger contracts.Logger) {
	m.logger = logger
}
