package foundation

import (
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/genesysflow/go-genesys/container"
	"github.com/genesysflow/go-genesys/contracts"
	"github.com/samber/do/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// deferredProbe is a deferrable provider that records its lifecycle.
type deferredProbe struct {
	registered bool
	booted     bool
}

func (p *deferredProbe) Register(app contracts.Application) error {
	p.registered = true
	return app.BindValue("probe.service", "ready")
}

func (p *deferredProbe) Boot(app contracts.Application) error {
	if !p.registered {
		panic("Boot ran before Register - deferred state is half-initialized")
	}
	p.booted = true
	return nil
}

func (p *deferredProbe) Provides() []string { return []string{"probe.service"} }
func (p *deferredProbe) IsDeferred() bool   { return true }

// A deferred provider must stay dormant through Boot() and come alive -
// Register first, then Boot - when its service is first resolved.
func TestDeferredProviderLifecycle(t *testing.T) {
	app := New(t.TempDir())
	probe := &deferredProbe{}
	require.NoError(t, app.Register(probe))

	require.NoError(t, app.Boot())
	assert.False(t, probe.registered, "deferred Register must not run at Boot")
	assert.False(t, probe.booted, "deferred Boot must not run at Boot")

	service, err := app.Make("probe.service")
	require.NoError(t, err)
	assert.Equal(t, "ready", service)
	assert.True(t, probe.registered)
	assert.True(t, probe.booted, "deferral resolved: Register then Boot ran")
}

// Configuration must be readable during a provider's Register phase -
// Laravel loads configuration before any provider registers.
func TestConfigAvailableDuringRegister(t *testing.T) {
	base := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(base, "config"), 0o755))
	// Files are keyed by filename: session.yaml holds the keys under
	// "session." directly.
	require.NoError(t, os.WriteFile(
		filepath.Join(base, "config", "session.yaml"),
		[]byte("driver: database\n"), 0o600))

	app := New(base)

	seen := ""
	require.NoError(t, app.Register(&callbackProvider{register: func(a contracts.Application) error {
		seen = a.GetConfig().GetString("session.driver")
		return nil
	}}))
	assert.Equal(t, "database", seen, "Register must see config/ values")
}

type callbackProvider struct {
	register func(contracts.Application) error
}

func (p *callbackProvider) Register(app contracts.Application) error { return p.register(app) }
func (p *callbackProvider) Boot(contracts.Application) error         { return nil }
func (p *callbackProvider) Provides() []string                       { return nil }

// Resolving a service whose factory resolves its own dependency must
// not deadlock while another goroutine registers bindings.
func TestContainerNestedResolveUnderConcurrentWrites(t *testing.T) {
	app := New(t.TempDir())

	container.Provide[*innerDep](app.Container, func(*do.RootScope) (*innerDep, error) {
		return &innerDep{}, nil
	})
	innerName := container.GetTypeName(reflect.TypeOf((*innerDep)(nil)))
	require.NoError(t, app.Container.Bind("outer", func() (any, error) {
		// Nested resolution through the container while Make is active.
		dep, err := app.Container.Make(innerName)
		if err != nil {
			return nil, err
		}
		return &outerDep{inner: dep.(*innerDep)}, nil
	}))

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			if n%2 == 0 {
				_, err := app.Container.Make("outer")
				assert.NoError(t, err)
			} else {
				assert.NoError(t, app.Container.Instance("side", n))
			}
		}(i)
	}
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("container deadlocked: nested Make blocked behind a queued writer")
	}
}

type innerDep struct{}
type outerDep struct{ inner *innerDep }
