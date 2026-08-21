package cache

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"
)

// TaggedCache scopes cache entries under one or more tags - Laravel's
// Cache::tags. Flushing a tag makes every entry stored under it
// unreachable, without touching entries under other tags:
//
//	users := cache.Tags(store, "users")
//	users.Put("count", 42, time.Minute)
//	cache.Tags(store, "users").Flush() // "count" is gone
//
// The implementation is version-keyed: each tag has a version counter
// in the store, and entry keys embed the current versions. Flushing
// increments the versions, orphaning old entries (they expire via their
// TTL, like Laravel's tagged cache). Because versions live in the store
// itself, tags work across processes on shared stores such as Redis.
type TaggedCache struct {
	store Store
	tags  []string
}

// Tags returns a tagged view of the store.
func Tags(store Store, tags ...string) *TaggedCache {
	sorted := append([]string(nil), tags...)
	sort.Strings(sorted)
	return &TaggedCache{store: store, tags: sorted}
}

func tagVersionKey(tag string) string { return "tag:" + tag + ":version" }

// namespace derives the current composite namespace from the tags'
// versions.
func (t *TaggedCache) namespace() (string, error) {
	parts := make([]string, len(t.tags))
	for i, tag := range t.tags {
		version, err := t.store.Get(tagVersionKey(tag))
		if err != nil {
			return "", err
		}
		parts[i] = fmt.Sprintf("%s=%v", tag, version)
	}
	sum := sha1.Sum([]byte(strings.Join(parts, "|")))
	return "tags:" + hex.EncodeToString(sum[:8]) + ":", nil
}

func (t *TaggedCache) key(key string) (string, error) {
	ns, err := t.namespace()
	if err != nil {
		return "", err
	}
	return ns + key, nil
}

// Get retrieves a tagged item (nil when missing, expired, or flushed).
func (t *TaggedCache) Get(key string) (any, error) {
	k, err := t.key(key)
	if err != nil {
		return nil, err
	}
	return t.store.Get(k)
}

// Put stores a tagged item.
func (t *TaggedCache) Put(key string, value any, ttl time.Duration) error {
	k, err := t.key(key)
	if err != nil {
		return err
	}
	return t.store.Put(k, value, ttl)
}

// Has reports whether a tagged item exists.
func (t *TaggedCache) Has(key string) (bool, error) {
	k, err := t.key(key)
	if err != nil {
		return false, err
	}
	return t.store.Has(k)
}

// Add stores a tagged item only when absent.
func (t *TaggedCache) Add(key string, value any, ttl time.Duration) (bool, error) {
	k, err := t.key(key)
	if err != nil {
		return false, err
	}
	return t.store.Add(k, value, ttl)
}

// Forget removes one tagged item.
func (t *TaggedCache) Forget(key string) error {
	k, err := t.key(key)
	if err != nil {
		return err
	}
	return t.store.Forget(k)
}

// Forever stores a tagged item without expiry. Note that a later Flush
// still orphans it (it becomes unreachable but is not deleted), so
// prefer TTLs under tags.
func (t *TaggedCache) Forever(key string, value any) error {
	k, err := t.key(key)
	if err != nil {
		return err
	}
	return t.store.Forever(k, value)
}

// Remember returns the tagged value, computing and storing it when
// absent.
func (t *TaggedCache) Remember(key string, ttl time.Duration, fn func() (any, error)) (any, error) {
	k, err := t.key(key)
	if err != nil {
		return nil, err
	}
	return Remember(t.store, k, ttl, fn)
}

// Flush makes every entry under ANY of this view's tags unreachable by
// bumping the tags' versions.
func (t *TaggedCache) Flush() error {
	for _, tag := range t.tags {
		if _, err := t.store.Increment(tagVersionKey(tag), 1); err != nil {
			return err
		}
	}
	return nil
}
