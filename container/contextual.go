package container

import (
	"fmt"
	"sync"
)

// Contextual bindings - Laravel's when/needs/give. A consumer-specific
// override is registered once and consulted by MakeFor, which falls
// back to the regular binding when no override exists:
//
//	c.When("reports").Needs("filesystem.disk").Give(func() (any, error) {
//	    return s3Disk, nil
//	})
//
//	disk, _ := c.MakeFor("reports", "filesystem.disk") // the s3 disk
//	disk, _ := c.MakeFor("exports", "filesystem.disk") // the default binding

var (
	contextualMu sync.RWMutex
	contextual   = map[*Container]map[string]map[string]func() (any, error){}
)

// ContextualBinding is the fluent builder returned by When.
type ContextualBinding struct {
	container *Container
	consumer  string
	need      string
}

// When starts a contextual binding for the named consumer.
func (c *Container) When(consumer string) *ContextualBinding {
	return &ContextualBinding{container: c, consumer: consumer}
}

// Needs names the service the consumer resolves differently.
func (b *ContextualBinding) Needs(service string) *ContextualBinding {
	b.need = service
	return b
}

// Give installs the factory used when the consumer resolves the service.
func (b *ContextualBinding) Give(factory func() (any, error)) {
	if b.need == "" {
		panic("container: contextual binding needs a service - call Needs before Give")
	}
	contextualMu.Lock()
	defer contextualMu.Unlock()
	byConsumer, ok := contextual[b.container]
	if !ok {
		byConsumer = map[string]map[string]func() (any, error){}
		contextual[b.container] = byConsumer
	}
	if byConsumer[b.consumer] == nil {
		byConsumer[b.consumer] = map[string]func() (any, error){}
	}
	byConsumer[b.consumer][b.need] = factory
}

// GiveValue installs a fixed value for the contextual binding.
func (b *ContextualBinding) GiveValue(value any) {
	b.Give(func() (any, error) { return value, nil })
}

// MakeFor resolves a service on behalf of a consumer, honouring
// contextual bindings and falling back to the regular container.
func (c *Container) MakeFor(consumer, name string) (any, error) {
	contextualMu.RLock()
	factory := contextual[c][consumer][name]
	contextualMu.RUnlock()
	if factory != nil {
		value, err := factory()
		if err != nil {
			return nil, fmt.Errorf("container: contextual binding %s->%s: %w", consumer, name, err)
		}
		return value, nil
	}
	return c.Make(name)
}
