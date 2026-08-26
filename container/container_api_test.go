package container

import (
	"context"
	"errors"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/samber/do/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- helper types -----------------------------------------------------------

// apiClient is the generic-API test subject; the pointer type gives us a
// stable identity to compare across resolutions.
type apiClient struct {
	endpoint string
	serial   int
}

type dbConn struct {
	dsn string
}

// plainShutdowner implements do.Shutdowner (Shutdown()).
type plainShutdowner struct {
	calls *int32
}

func (s *plainShutdowner) Shutdown() { atomic.AddInt32(s.calls, 1) }

// errShutdowner implements do.ShutdownerWithError (Shutdown() error).
type errShutdowner struct {
	calls *int32
	err   error
}

func (s *errShutdowner) Shutdown() error {
	atomic.AddInt32(s.calls, 1)
	return s.err
}

// ctxShutdowner implements do.ShutdownerWithContextAndError.
type ctxShutdowner struct {
	calls  *int32
	sawErr error
}

func (s *ctxShutdowner) Shutdown(ctx context.Context) error {
	atomic.AddInt32(s.calls, 1)
	s.sawErr = ctx.Err()
	return ctx.Err()
}

// --- Provide* ---------------------------------------------------------------

func TestProvide_IsSingletonAndLazy(t *testing.T) {
	c := New()
	calls := 0

	Provide(c, func(scope *do.RootScope) (*apiClient, error) {
		calls++
		// The factory is handed the container's own root scope.
		assert.Same(t, c.Injector(), scope)
		return &apiClient{endpoint: "https://example.test", serial: calls}, nil
	})

	// Lazy: nothing is built until the first resolution.
	assert.Equal(t, 0, calls)

	first, err := Invoke[*apiClient](c)
	require.NoError(t, err)
	second, err := Invoke[*apiClient](c)
	require.NoError(t, err)

	assert.Same(t, first, second, "Provide registers a singleton")
	assert.Equal(t, 1, calls, "the factory runs once")
	assert.Equal(t, "https://example.test", first.endpoint)
}

func TestProvide_FactoryErrorIsReturned(t *testing.T) {
	c := New()
	boom := errors.New("cannot dial")

	Provide(c, func(*do.RootScope) (*dbConn, error) {
		return nil, boom
	})

	got, err := Invoke[*dbConn](c)
	assert.Nil(t, got)
	require.Error(t, err)
	assert.ErrorIs(t, err, boom)
}

func TestProvideNamed_SingletonUnderName(t *testing.T) {
	c := New()
	calls := 0

	ProvideNamed(c, "primary", func(*do.RootScope) (*dbConn, error) {
		calls++
		return &dbConn{dsn: "primary.sqlite"}, nil
	})

	first, err := InvokeNamed[*dbConn](c, "primary")
	require.NoError(t, err)
	second, err := InvokeNamed[*dbConn](c, "primary")
	require.NoError(t, err)

	assert.Same(t, first, second)
	assert.Equal(t, 1, calls)

	// Named generic registrations are tracked by Has and are also
	// reachable through the untyped Make API.
	assert.True(t, c.Has("primary"))
	untyped, err := c.Make("primary")
	require.NoError(t, err)
	assert.Same(t, first, untyped)
}

func TestProvideNamed_DistinctNamesAreDistinctServices(t *testing.T) {
	c := New()

	ProvideNamed(c, "read", func(*do.RootScope) (*dbConn, error) { return &dbConn{dsn: "read"}, nil })
	ProvideNamed(c, "write", func(*do.RootScope) (*dbConn, error) { return &dbConn{dsn: "write"}, nil })

	read, err := InvokeNamed[*dbConn](c, "read")
	require.NoError(t, err)
	write, err := InvokeNamed[*dbConn](c, "write")
	require.NoError(t, err)

	assert.NotSame(t, read, write)
	assert.Equal(t, "read", read.dsn)
	assert.Equal(t, "write", write.dsn)
}

func TestProvideValue_ResolvesTheSameValue(t *testing.T) {
	c := New()
	value := &apiClient{endpoint: "value"}

	ProvideValue(c, value)

	first, err := Invoke[*apiClient](c)
	require.NoError(t, err)
	second, err := Invoke[*apiClient](c)
	require.NoError(t, err)

	assert.Same(t, value, first)
	assert.Same(t, value, second)

	// The unnamed generic helpers register under the inferred type name,
	// and Has agrees with Make about it.
	name := GetTypeName(reflect.TypeOf(value))
	assert.True(t, c.Has(name), "Has knows about unnamed generic registrations")
	viaMake, err := c.Make(name)
	require.NoError(t, err, "Make resolves them by inferred type name")
	assert.Same(t, value, viaMake)
}

func TestProvideNamedValue_ResolvesAndIsTracked(t *testing.T) {
	c := New()
	value := &dbConn{dsn: "named-value"}

	ProvideNamedValue(c, "cache", value)

	resolved, err := InvokeNamed[*dbConn](c, "cache")
	require.NoError(t, err)
	assert.Same(t, value, resolved)
	assert.True(t, c.Has("cache"))
}

func TestProvideTransient_NewInstanceEachResolution(t *testing.T) {
	c := New()
	calls := 0

	ProvideTransient(c, func(*do.RootScope) (*apiClient, error) {
		calls++
		return &apiClient{endpoint: "transient", serial: calls}, nil
	})

	first, err := Invoke[*apiClient](c)
	require.NoError(t, err)
	second, err := Invoke[*apiClient](c)
	require.NoError(t, err)

	assert.NotSame(t, first, second, "ProvideTransient builds a fresh instance per resolution")
	assert.Equal(t, 1, first.serial)
	assert.Equal(t, 2, second.serial)
	assert.Equal(t, 2, calls)
}

func TestProvideNamedTransient_NewInstanceEachResolution(t *testing.T) {
	c := New()
	calls := 0

	ProvideNamedTransient(c, "session", func(*do.RootScope) (*dbConn, error) {
		calls++
		return &dbConn{dsn: "session"}, nil
	})

	first, err := InvokeNamed[*dbConn](c, "session")
	require.NoError(t, err)
	second, err := InvokeNamed[*dbConn](c, "session")
	require.NoError(t, err)

	assert.NotSame(t, first, second)
	assert.Equal(t, 2, calls)
	assert.True(t, c.Has("session"))
}

// --- Invoke* ----------------------------------------------------------------

func TestInvoke_MissingServiceReturnsError(t *testing.T) {
	c := New()

	got, err := Invoke[*apiClient](c)
	assert.Nil(t, got)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not find service")
}

func TestMustInvoke_SuccessAndPanic(t *testing.T) {
	c := New()
	value := &apiClient{endpoint: "must"}
	ProvideValue(c, value)

	assert.Same(t, value, MustInvoke[*apiClient](c))

	// A type nobody registered blows up.
	assert.Panics(t, func() {
		MustInvoke[*dbConn](c)
	})
}

func TestInvokeNamed_MissingNameReturnsError(t *testing.T) {
	c := New()
	ProvideNamedValue(c, "known", &dbConn{dsn: "known"})

	got, err := InvokeNamed[*dbConn](c, "unknown")
	assert.Nil(t, got)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown")
}

func TestMustInvokeNamed_SuccessAndPanic(t *testing.T) {
	c := New()
	value := &dbConn{dsn: "must-named"}
	ProvideNamedValue(c, "must", value)

	assert.Same(t, value, MustInvokeNamed[*dbConn](c, "must"))

	assert.Panics(t, func() {
		MustInvokeNamed[*dbConn](c, "missing")
	})
}

func TestInvokeNamed_DependencyResolvedInsideFactory(t *testing.T) {
	c := New()

	ProvideNamedValue(c, "dsn", "postgres://localhost/app")

	// A factory can pull its own dependencies off the injector it is given.
	ProvideNamed(c, "db", func(scope *do.RootScope) (*dbConn, error) {
		dsn, err := do.InvokeNamed[string](scope, "dsn")
		if err != nil {
			return nil, err
		}
		return &dbConn{dsn: dsn}, nil
	})

	conn, err := InvokeNamed[*dbConn](c, "db")
	require.NoError(t, err)
	assert.Equal(t, "postgres://localhost/app", conn.dsn)
}

func TestInvokeNamed_MissingDependencyInsideFactory(t *testing.T) {
	c := New()

	ProvideNamed(c, "db", func(scope *do.RootScope) (*dbConn, error) {
		dsn, err := do.InvokeNamed[string](scope, "dsn")
		if err != nil {
			return nil, err
		}
		return &dbConn{dsn: dsn}, nil
	})

	got, err := InvokeNamed[*dbConn](c, "db")
	assert.Nil(t, got)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dsn")
}

// --- Override* --------------------------------------------------------------

func TestOverride_ReplacesExistingFactory(t *testing.T) {
	c := New()

	Provide(c, func(*do.RootScope) (*apiClient, error) {
		return &apiClient{endpoint: "original"}, nil
	})
	original, err := Invoke[*apiClient](c)
	require.NoError(t, err)
	require.Equal(t, "original", original.endpoint)

	Override(c, func(*do.RootScope) (*apiClient, error) {
		return &apiClient{endpoint: "replacement"}, nil
	})

	replaced, err := Invoke[*apiClient](c)
	require.NoError(t, err)
	assert.Equal(t, "replacement", replaced.endpoint)
	assert.NotSame(t, original, replaced)
}

func TestOverride_WithoutExistingBindingActsLikeProvide(t *testing.T) {
	c := New()

	// do's Override intentionally does not require a prior registration.
	assert.NotPanics(t, func() {
		Override(c, func(*do.RootScope) (*apiClient, error) {
			return &apiClient{endpoint: "fresh"}, nil
		})
	})

	got, err := Invoke[*apiClient](c)
	require.NoError(t, err)
	assert.Equal(t, "fresh", got.endpoint)
}

func TestOverrideNamed_ReplacesExistingNamedFactory(t *testing.T) {
	c := New()

	ProvideNamed(c, "queue", func(*do.RootScope) (*dbConn, error) {
		return &dbConn{dsn: "sync"}, nil
	})
	original, err := InvokeNamed[*dbConn](c, "queue")
	require.NoError(t, err)
	require.Equal(t, "sync", original.dsn)

	OverrideNamed(c, "queue", func(*do.RootScope) (*dbConn, error) {
		return &dbConn{dsn: "redis"}, nil
	})

	replaced, err := InvokeNamed[*dbConn](c, "queue")
	require.NoError(t, err)
	assert.Equal(t, "redis", replaced.dsn)
}

func TestOverrideNamed_WithoutExistingBindingRegistersIt(t *testing.T) {
	c := New()

	assert.NotPanics(t, func() {
		OverrideNamed(c, "brand-new", func(*do.RootScope) (*dbConn, error) {
			return &dbConn{dsn: "brand-new"}, nil
		})
	})

	got, err := InvokeNamed[*dbConn](c, "brand-new")
	require.NoError(t, err)
	assert.Equal(t, "brand-new", got.dsn)

	// Override* registers the name, so Has agrees the service exists.
	assert.True(t, c.Has("brand-new"))
}

func TestOverrideValue_ReplacesExistingValue(t *testing.T) {
	c := New()
	first := &apiClient{endpoint: "first"}
	second := &apiClient{endpoint: "second"}

	ProvideValue(c, first)
	OverrideValue(c, second)

	got, err := Invoke[*apiClient](c)
	require.NoError(t, err)
	assert.Same(t, second, got)
}

func TestOverrideNamedValue_ReplacesExistingNamedValue(t *testing.T) {
	c := New()
	first := &dbConn{dsn: "first"}
	second := &dbConn{dsn: "second"}

	ProvideNamedValue(c, "conn", first)
	resolved, err := InvokeNamed[*dbConn](c, "conn")
	require.NoError(t, err)
	require.Same(t, first, resolved)

	OverrideNamedValue(c, "conn", second)

	resolved, err = InvokeNamed[*dbConn](c, "conn")
	require.NoError(t, err)
	assert.Same(t, second, resolved)
}

// --- Has / MustMake ---------------------------------------------------------

func TestHas(t *testing.T) {
	c := New()

	assert.False(t, c.Has("nothing"))

	require.NoError(t, c.Bind("bound", func() (string, error) { return "bound", nil }))
	assert.True(t, c.Has("bound"))

	require.NoError(t, c.Singleton("shared", func() (string, error) { return "shared", nil }))
	assert.True(t, c.Has("shared"))

	require.NoError(t, c.Instance("instance", &dbConn{dsn: "instance"}))
	assert.True(t, c.Has("instance"))

	ProvideNamedValue(c, "named-value", &dbConn{dsn: "nv"})
	assert.True(t, c.Has("named-value"))

	assert.False(t, c.Has("still-nothing"))
}

func TestMustMake_ReturnsService(t *testing.T) {
	c := New()
	svc := &dbConn{dsn: "must-make"}
	require.NoError(t, c.Instance("db", svc))

	assert.Same(t, svc, c.MustMake("db"))
}

func TestMustMake_PanicsWhenMissing(t *testing.T) {
	c := New()

	assert.PanicsWithValue(t, "container: failed to resolve service 'ghost': DI: could not find service `ghost`, no service available", func() {
		c.MustMake("ghost")
	})
}

func TestMustMake_PanicsWhenFactoryFails(t *testing.T) {
	c := New()
	require.NoError(t, c.Singleton("broken", func() (*dbConn, error) {
		return nil, errors.New("factory exploded")
	}))

	assert.Panics(t, func() {
		c.MustMake("broken")
	})
}

// --- Shutdown ---------------------------------------------------------------

func TestShutdown_RunsHooksOnInstantiatedServices(t *testing.T) {
	c := New()
	var eager, lazyUsed, lazyUnused int32

	// Eager values are always shut down.
	ProvideNamedValue(c, "eager", &plainShutdowner{calls: &eager})
	// Lazy services are only shut down once they have been built.
	ProvideNamed(c, "lazy-used", func(*do.RootScope) (*errShutdowner, error) {
		return &errShutdowner{calls: &lazyUsed}, nil
	})
	ProvideNamed(c, "lazy-unused", func(*do.RootScope) (*plainShutdowner, error) {
		return &plainShutdowner{calls: &lazyUnused}, nil
	})

	_, err := InvokeNamed[*errShutdowner](c, "lazy-used")
	require.NoError(t, err)

	_ = c.Shutdown()

	assert.Equal(t, int32(1), atomic.LoadInt32(&eager), "eager value shutdown hook ran")
	assert.Equal(t, int32(1), atomic.LoadInt32(&lazyUsed), "instantiated lazy shutdown hook ran")
	assert.Equal(t, int32(0), atomic.LoadInt32(&lazyUnused), "never-built service is not shut down")
}

func TestShutdown_ReturnsNilOnSuccess(t *testing.T) {
	c := New()
	var calls int32
	ProvideNamedValue(c, "svc", &plainShutdowner{calls: &calls})

	err := c.Shutdown()

	// do hands back a non-nil *ShutdownReport even when nothing failed;
	// Container.Shutdown unwraps it so the idiomatic
	// `if err := c.Shutdown(); err != nil` only fires on real failures.
	assert.NoError(t, err, "a fully successful shutdown reports no error")
	assert.Equal(t, int32(1), atomic.LoadInt32(&calls))
}

func TestShutdown_EmptyContainerReturnsNil(t *testing.T) {
	c := New()

	assert.NoError(t, c.Shutdown(), "a container with zero bindings shuts down cleanly")
}

func TestShutdown_CollectsServiceErrors(t *testing.T) {
	c := New()
	var calls int32
	failure := errors.New("could not close connection")
	ProvideNamedValue(c, "flaky", &errShutdowner{calls: &calls, err: failure})

	err := c.Shutdown()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not close connection")
	assert.Equal(t, int32(1), atomic.LoadInt32(&calls))

	report, ok := err.(*do.ShutdownReport)
	require.True(t, ok)
	assert.False(t, report.Succeed)
	assert.Len(t, report.Errors, 1)
}

func TestShutdownWithContext_PassesContextToHooks(t *testing.T) {
	c := New()
	var calls int32
	svc := &ctxShutdowner{calls: &calls}
	ProvideNamedValue(c, "ctx-svc", svc)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := c.ShutdownWithContext(ctx)

	assert.Equal(t, int32(1), atomic.LoadInt32(&calls))
	assert.NoError(t, svc.sawErr, "the live context is handed to the hook")
	assert.NoError(t, err)
}

func TestShutdownWithContext_CancelledContext(t *testing.T) {
	c := New()
	var calls int32
	svc := &ctxShutdowner{calls: &calls}
	ProvideNamedValue(c, "ctx-svc", svc)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := c.ShutdownWithContext(ctx)

	// do short-circuits on an already-cancelled context: the hook never
	// runs and the cancellation is reported as the service's error.
	assert.Equal(t, int32(0), atomic.LoadInt32(&calls))
	require.Error(t, err)
	assert.Contains(t, err.Error(), context.Canceled.Error())

	report, ok := err.(*do.ShutdownReport)
	require.True(t, ok)
	assert.False(t, report.Succeed)
	assert.Len(t, report.Errors, 1)
	for _, serviceErr := range report.Errors {
		assert.ErrorIs(t, serviceErr, context.Canceled)
	}
}

func TestShutdownWithContext_ExpiredDeadline(t *testing.T) {
	c := New()
	var calls int32
	ProvideNamedValue(c, "ctx-svc", &ctxShutdowner{calls: &calls})

	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	<-ctx.Done()

	err := c.ShutdownWithContext(ctx)

	assert.Equal(t, int32(0), atomic.LoadInt32(&calls))
	require.Error(t, err)
	assert.Contains(t, err.Error(), context.DeadlineExceeded.Error())
}

func TestShutdown_ServiceIsGoneAfterwards(t *testing.T) {
	c := New()
	ProvideNamedValue(c, "temp", &dbConn{dsn: "temp"})

	require.True(t, c.Has("temp"))
	_ = c.Shutdown()

	_, err := InvokeNamed[*dbConn](c, "temp")
	require.Error(t, err, "the binding is removed from the injector")

	// Has follows the injector, so it stops claiming the service exists
	// once it has been torn down.
	assert.False(t, c.Has("temp"))
}

// --- remaining branches of the non-generic API ------------------------------

func TestSingleton_RebindReplacesPreviousFactory(t *testing.T) {
	c := New()

	require.NoError(t, c.Singleton("cfg", func() (*dbConn, error) { return &dbConn{dsn: "one"}, nil }))
	first, err := c.Make("cfg")
	require.NoError(t, err)
	require.Equal(t, "one", first.(*dbConn).dsn)

	// Re-registering an existing name goes through do.OverrideNamed.
	require.NoError(t, c.Singleton("cfg", func() (*dbConn, error) { return &dbConn{dsn: "two"}, nil }))
	second, err := c.Make("cfg")
	require.NoError(t, err)
	assert.Equal(t, "two", second.(*dbConn).dsn)

	third, err := c.Make("cfg")
	require.NoError(t, err)
	assert.Same(t, second, third, "the replacement is still a singleton")
}

func TestSingletonType_RejectsNonFunction(t *testing.T) {
	c := New()

	err := c.SingletonType(42)
	require.Error(t, err)
	assert.Equal(t, "container: factory must be a function", err.Error())
}

func TestInstance_RebindReplacesPreviousInstance(t *testing.T) {
	c := New()
	first := &dbConn{dsn: "first"}
	second := &dbConn{dsn: "second"}

	require.NoError(t, c.Instance("conn", first))
	require.NoError(t, c.Instance("conn", second))

	resolved, err := c.Make("conn")
	require.NoError(t, err)
	assert.Same(t, second, resolved)
}

func TestBind_NonFunctionFactoryYieldsTheValueItself(t *testing.T) {
	c := New()

	require.NoError(t, c.Bind("literal", "just-a-string"))

	got, err := c.Make("literal")
	require.NoError(t, err)
	assert.Equal(t, "just-a-string", got)
}

func TestBind_FactoryReceivesTheContainer(t *testing.T) {
	c := New()
	require.NoError(t, c.Instance("dsn", "sqlite://memory"))

	require.NoError(t, c.Singleton("db", func(inner *Container) (*dbConn, error) {
		dsn, err := inner.Make("dsn")
		if err != nil {
			return nil, err
		}
		return &dbConn{dsn: dsn.(string)}, nil
	}))

	got, err := c.Make("db")
	require.NoError(t, err)
	assert.Equal(t, "sqlite://memory", got.(*dbConn).dsn)
}

func TestBind_FactoryReturningOnlyAnError(t *testing.T) {
	c := New()
	boom := errors.New("nope")

	require.NoError(t, c.Singleton("err-only", func() error { return boom }))
	_, err := c.Make("err-only")
	require.Error(t, err)
	assert.ErrorIs(t, err, boom)

	require.NoError(t, c.Singleton("nil-err", func() error { return nil }))
	got, err := c.Make("nil-err")
	require.NoError(t, err)
	assert.Nil(t, got, "a lone nil error resolves to a nil service")
}

func TestCall_ReturnsErrorForNonFunction(t *testing.T) {
	c := New()

	got, err := c.Call("not a function")
	assert.Nil(t, got)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Call expected a function")
}

func TestCall_FunctionWithoutReturnValues(t *testing.T) {
	c := New()
	called := false

	results, err := c.Call(func() { called = true })
	require.NoError(t, err)
	assert.True(t, called)
	assert.Equal(t, []any{nil}, results)
}

func TestGetTypeName_PointerToBuiltin(t *testing.T) {
	assert.Equal(t, "*int", GetTypeName(reflect.TypeOf(new(int))))
	assert.Equal(t, "[]string", GetTypeName(reflect.TypeOf([]string{})))
}

func TestResolve_TypeMismatch(t *testing.T) {
	c := New()
	require.NoError(t, c.Instance("thing", &dbConn{dsn: "thing"}))

	got, err := Resolve[*apiClient](c, "thing")
	assert.Nil(t, got)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is not of type")
}

func TestMustResolve_ReturnsService(t *testing.T) {
	c := New()
	svc := &dbConn{dsn: "must-resolve"}
	require.NoError(t, c.Instance("db", svc))

	assert.Same(t, svc, MustResolve[*dbConn](c, "db"))
}

func TestContextualBinding_GiveWithoutNeedsPanics(t *testing.T) {
	c := New()

	assert.PanicsWithValue(t, "container: contextual binding needs a service - call Needs before Give", func() {
		c.When("reports").Give(func() (any, error) { return nil, nil })
	})
}

func TestMakeFor_FactoryErrorIsWrapped(t *testing.T) {
	c := New()
	boom := errors.New("disk unavailable")
	c.When("reports").Needs("disk").Give(func() (any, error) { return nil, boom })

	got, err := c.MakeFor("reports", "disk")
	assert.Nil(t, got)
	require.Error(t, err)
	assert.ErrorIs(t, err, boom)
	assert.Contains(t, err.Error(), "contextual binding reports->disk")
}
