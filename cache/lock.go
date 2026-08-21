package cache

import (
	"crypto/rand"
	"encoding/hex"
	"time"
)

// Lock is an atomic lock built on a cache store, Laravel's Cache::lock.
// Acquisition uses the store's atomic Add, so two processes sharing a
// store (e.g. Redis) cannot both hold the same lock. Release only
// removes the lock when this instance still owns it.
type Lock struct {
	store Store
	name  string
	ttl   time.Duration
	owner string
}

// NewLock creates a lock handle. The lock is not acquired until Get or
// Block succeeds; ttl bounds how long a crashed holder can wedge it.
func NewLock(store Store, name string, ttl time.Duration) *Lock {
	token := make([]byte, 16)
	rand.Read(token)
	return &Lock{
		store: store,
		name:  "lock:" + name,
		ttl:   ttl,
		owner: hex.EncodeToString(token),
	}
}

// Get attempts to acquire the lock, reporting whether it succeeded.
func (l *Lock) Get() (bool, error) {
	return l.store.Add(l.name, l.owner, l.ttl)
}

// Block polls for the lock until it is acquired or the timeout elapses.
func (l *Lock) Block(timeout time.Duration) (bool, error) {
	deadline := time.Now().Add(timeout)
	for {
		acquired, err := l.Get()
		if err != nil || acquired {
			return acquired, err
		}
		if !time.Now().Before(deadline) {
			return false, nil
		}
		time.Sleep(25 * time.Millisecond)
	}
}

// conditionalForgetter is implemented by stores that can atomically
// delete a key only while it still holds the given value (Laravel's
// Lua-scripted lock release). The memory and redis stores implement it.
type conditionalForgetter interface {
	ForgetIfEquals(key string, value string) (bool, error)
}

// Release frees the lock when this instance owns it, reporting whether
// anything was released. On stores supporting an atomic
// compare-and-delete the release cannot free a lock that expired and
// was re-acquired by someone else in between.
func (l *Lock) Release() (bool, error) {
	if store, ok := l.store.(conditionalForgetter); ok {
		return store.ForgetIfEquals(l.name, l.owner)
	}
	// Fallback for stores without compare-and-delete: a narrow race
	// remains between the ownership check and the delete.
	current, err := l.store.Get(l.name)
	if err != nil {
		return false, err
	}
	if owner, ok := current.(string); !ok || owner != l.owner {
		return false, nil // expired, or stolen after expiry - not ours to free
	}
	if err := l.store.Forget(l.name); err != nil {
		return false, err
	}
	return true, nil
}

// ForceRelease frees the lock regardless of who owns it.
func (l *Lock) ForceRelease() error {
	return l.store.Forget(l.name)
}

// WithLock acquires the lock (blocking up to wait), runs fn while
// holding it, and releases it afterwards. Reports false without running
// fn when the lock could not be acquired in time.
func WithLock(store Store, name string, ttl, wait time.Duration, fn func() error) (bool, error) {
	lock := NewLock(store, name, ttl)
	acquired, err := lock.Block(wait)
	if err != nil || !acquired {
		return false, err
	}
	defer lock.Release()
	return true, fn()
}
