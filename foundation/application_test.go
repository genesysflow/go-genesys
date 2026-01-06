package foundation

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/genesysflow/go-genesys/container"
	"github.com/genesysflow/go-genesys/contracts"
	"github.com/samber/do/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockProvider is a mock service provider
type MockProvider struct {
	mock.Mock
}

func (m *MockProvider) Register(app contracts.Application) error {
	args := m.Called(app)
	return args.Error(0)
}

func (m *MockProvider) Boot(app contracts.Application) error {
	args := m.Called(app)
	return args.Error(0)
}

func (m *MockProvider) Provides() []string {
	return []string{}
}

func TestNew_WithBasePath(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "app_test")
	defer os.RemoveAll(tmpDir)

	app := New(tmpDir)
	assert.Equal(t, tmpDir, app.BasePath())
	assert.Equal(t, filepath.Join(tmpDir, "config"), app.ConfigPath())
}

func TestNew_DefaultPath(t *testing.T) {
	app := New()
	cwd, _ := os.Getwd()
	assert.Equal(t, cwd, app.BasePath())
}

func TestRegister(t *testing.T) {
	app := New()

	provider := new(MockProvider)
	provider.On("Register", app).Return(nil)

	err := app.Register(provider)
	assert.NoError(t, err)

	provider.AssertExpectations(t)
}

func TestBoot(t *testing.T) {
	app := New()

	provider := new(MockProvider)
	provider.On("Register", app).Return(nil)
	provider.On("Boot", app).Return(nil)

	err := app.Register(provider)
	assert.NoError(t, err)

	err = app.Boot()
	assert.NoError(t, err)
	assert.True(t, app.IsBooted())

	provider.AssertExpectations(t)
}

func TestAliases(t *testing.T) {
	app := New()

	// Check core aliases
	assert.True(t, app.Has("app"))
	assert.True(t, app.Has("config"))
	assert.True(t, app.Has("logger"))
}

func TestEnvironment(t *testing.T) {
	os.Setenv("APP_ENV", "production")
	defer os.Unsetenv("APP_ENV")

	app := New()
	assert.Equal(t, "production", app.Environment())
	assert.True(t, app.IsProduction())
	assert.False(t, app.IsLocal())
}

func TestCallbacks(t *testing.T) {
	app := New()
	bootingCalled := false
	bootedCalled := false

	app.Booting(func(a contracts.Application) {
		bootingCalled = true
	})

	app.Booted(func(a contracts.Application) {
		bootedCalled = true
	})

	app.Boot()
	assert.True(t, bootingCalled)
	assert.True(t, bootedCalled)
}

func TestTerminate(t *testing.T) {
	app := New()
	terminatedCalled := false

	app.Terminating(func(a contracts.Application) {
		terminatedCalled = true
	})

	err := app.Terminate()
	if err != nil {
		assert.Contains(t, err.Error(), "shutdown errors")
	}
	assert.True(t, terminatedCalled)
}

// verify interfaces
var _ contracts.Application = (*Application)(nil)
var _ contracts.Container = (*Application)(nil)

func TestMakeAndProvide(t *testing.T) {
	app := New()

	// Test binding and making
	type Config struct {
		Foo string
	}

	Provide(app, func(injector *do.RootScope) (*Config, error) {
		return &Config{Foo: "bar"}, nil
	})

	// Resolve
	instance := container.MustResolve[*Config](app)
	assert.Equal(t, "bar", instance.Foo)

	// Test MustMake
	instance2 := container.MustInvoke[*Config](app.Container)
	assert.Equal(t, "bar", instance2.Foo)
}

func TestResolve_WithApplication(t *testing.T) {
	// This tests the `Resolve` function which takes contracts.Container.
	// Application implements it.
	app := New()

	ProvideValue(app, "hello")

	val, err := container.Resolve[string](app)
	assert.NoError(t, err)
	assert.Equal(t, "hello", val)
}
