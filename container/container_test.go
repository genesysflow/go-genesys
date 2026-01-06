package container

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

type TestService interface {
	GetValue() string
}

type testServiceImpl struct {
	Value string
}

func (t *testServiceImpl) GetValue() string {
	return t.Value
}

type AnotherService struct {
	Service TestService
}

func TestNew(t *testing.T) {
	c := New()
	assert.NotNil(t, c)
	assert.NotNil(t, c.Injector())
}

func TestBind(t *testing.T) {
	c := New()

	// Test basic binding
	err := c.Bind("service", func() (TestService, error) {
		return &testServiceImpl{Value: "test"}, nil
	})
	assert.NoError(t, err)

	// Test resolution
	instance, err := c.Make("service")
	assert.NoError(t, err)
	assert.IsType(t, &testServiceImpl{}, instance)
	assert.Equal(t, "test", instance.(TestService).GetValue())
	instance2, _ := c.Make("service")
	assert.NotSame(t, instance, instance2)
}

func TestSingleton(t *testing.T) {
	c := New()

	err := c.Singleton("singleton", func() (TestService, error) {
		return &testServiceImpl{Value: "singleton"}, nil
	})
	assert.NoError(t, err)

	instance1, _ := c.Make("singleton")
	instance2, _ := c.Make("singleton")

	assert.Same(t, instance1, instance2)
}

func TestBindType(t *testing.T) {
	c := New()

	err := c.BindType(func() (*testServiceImpl, error) {
		return &testServiceImpl{Value: "typed"}, nil
	})
	assert.NoError(t, err)

	// Should be able to resolve by string name of the type
	// GetTypeName returns pointer type string like "*container.testServiceImpl"
	// But since we are in the same package, it might be slightly different depending on how GetTypeName works.
	// Let's rely on Resolve[T] mostly, but here we test the internal naming.
	// The implementation of BindType uses inferServiceName.

	// Let's just use Resolve to verify it works
	instance, err := Resolve[*testServiceImpl](c)
	assert.NoError(t, err)
	assert.Equal(t, "typed", instance.Value)
}

func TestInstance(t *testing.T) {
	c := New()
	svc := &testServiceImpl{Value: "instance"}

	err := c.Instance("myinstance", svc)
	assert.NoError(t, err)

	resolved, err := c.Make("myinstance")
	assert.NoError(t, err)
	assert.Same(t, svc, resolved)
}

func TestInstanceType(t *testing.T) {
	c := New()
	svc := &testServiceImpl{Value: "instancetype"}

	err := c.InstanceType(svc)
	assert.NoError(t, err)

	resolved, err := Resolve[*testServiceImpl](c)
	assert.NoError(t, err)
	assert.Same(t, svc, resolved)
}

func TestCall(t *testing.T) {
	c := New()

	c.Instance("dependency", &testServiceImpl{Value: "dep"})

	// Function to call
	myFunc := func(svc TestService) string {
		return svc.GetValue() + "_handled"
	}

	interfaceType := GetTypeName(reflect.TypeOf((*TestService)(nil)).Elem())
	c.Instance(interfaceType, &testServiceImpl{Value: "injected"})

	results, err := c.Call(myFunc)
	assert.NoError(t, err)
	assert.Equal(t, "injected_handled", results[0])
}

func TestInvokeFactoryErrors(t *testing.T) {
	c := New()

	// Function needing a missing dependency
	myFunc := func(svc TestService) {}

	_, err := c.Call(myFunc)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to resolve dependency")
}

func TestResolve(t *testing.T) {
	c := New()
	expected := &testServiceImpl{Value: "resolved"}
	c.SingletonType(func() (*testServiceImpl, error) {
		return expected, nil
	})

	// Test Resolve with generics
	res, err := Resolve[*testServiceImpl](c)
	assert.NoError(t, err)
	assert.Same(t, expected, res)

	// Test Resolve with explicit name
	c.Instance("named_svc", expected)
	resNamed, err := Resolve[*testServiceImpl](c, "named_svc")
	assert.NoError(t, err)
	assert.Same(t, expected, resNamed)
}

func TestMustResolve_PanicsOnError(t *testing.T) {
	c := New()

	assert.Panics(t, func() {
		MustResolve[*testServiceImpl](c)
	})
}

func TestGetTypeName(t *testing.T) {
	// Basic types
	assert.Equal(t, "int", GetTypeName(reflect.TypeOf(int(0))))
	assert.Equal(t, "string", GetTypeName(reflect.TypeOf("")))

	// Structs
	assert.Contains(t, GetTypeName(reflect.TypeOf(testServiceImpl{})), "container.testServiceImpl")

	// Pointers
	ptrName := GetTypeName(reflect.TypeOf(&testServiceImpl{}))
	assert.Contains(t, ptrName, "*")
	assert.Contains(t, ptrName, "container.testServiceImpl")
}

func TestInferServiceName_Error(t *testing.T) {
	c := New()

	// Not a function
	err := c.BindType("not a function")
	assert.Error(t, err)
	assert.Equal(t, "container: factory must be a function", err.Error())

	// No return values
	err = c.BindType(func() {})
	assert.Error(t, err)
	assert.Equal(t, "container: factory must return at least one value", err.Error())
}

func TestOverride(t *testing.T) {
	c := New()

	// Initial binding
	c.Bind("svc", func() (string, error) { return "initial", nil })

	// Override
	c.Bind("svc", func() (string, error) { return "overridden", nil })

	res, err := c.Make("svc")
	assert.NoError(t, err)
	assert.Equal(t, "overridden", res)
}
