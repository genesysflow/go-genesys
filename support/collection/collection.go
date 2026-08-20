// Package collection provides a Laravel-style fluent collection type
// built on Go generics.
package collection

import (
	"sort"
)

// Collection wraps a slice with fluent transformation helpers.
type Collection[T any] struct {
	items []T
}

// Collect creates a collection from the given items.
func Collect[T any](items ...T) *Collection[T] {
	return &Collection[T]{items: items}
}

// FromSlice creates a collection wrapping the given slice.
func FromSlice[T any](items []T) *Collection[T] {
	return &Collection[T]{items: items}
}

// All returns the underlying slice.
func (c *Collection[T]) All() []T {
	return c.items
}

// Len returns the number of items.
func (c *Collection[T]) Len() int {
	return len(c.items)
}

// IsEmpty reports whether the collection has no items.
func (c *Collection[T]) IsEmpty() bool {
	return len(c.items) == 0
}

// First returns the first item, or the zero value when empty.
func (c *Collection[T]) First() T {
	var zero T
	if len(c.items) == 0 {
		return zero
	}
	return c.items[0]
}

// FirstWhere returns the first item matching the predicate and whether one was found.
func (c *Collection[T]) FirstWhere(fn func(T) bool) (T, bool) {
	for _, item := range c.items {
		if fn(item) {
			return item, true
		}
	}
	var zero T
	return zero, false
}

// Last returns the last item, or the zero value when empty.
func (c *Collection[T]) Last() T {
	var zero T
	if len(c.items) == 0 {
		return zero
	}
	return c.items[len(c.items)-1]
}

// Each calls fn for every item and returns the collection.
func (c *Collection[T]) Each(fn func(T)) *Collection[T] {
	for _, item := range c.items {
		fn(item)
	}
	return c
}

// Filter returns a new collection with the items the predicate accepts.
func (c *Collection[T]) Filter(fn func(T) bool) *Collection[T] {
	out := make([]T, 0, len(c.items))
	for _, item := range c.items {
		if fn(item) {
			out = append(out, item)
		}
	}
	return FromSlice(out)
}

// Reject returns a new collection without the items the predicate accepts.
func (c *Collection[T]) Reject(fn func(T) bool) *Collection[T] {
	return c.Filter(func(item T) bool { return !fn(item) })
}

// Map transforms every item with fn (same element type; use collection.Map
// for type-changing transforms).
func (c *Collection[T]) Map(fn func(T) T) *Collection[T] {
	out := make([]T, len(c.items))
	for i, item := range c.items {
		out[i] = fn(item)
	}
	return FromSlice(out)
}

// Contains reports whether any item matches the predicate.
func (c *Collection[T]) Contains(fn func(T) bool) bool {
	_, found := c.FirstWhere(fn)
	return found
}

// Every reports whether all items match the predicate.
func (c *Collection[T]) Every(fn func(T) bool) bool {
	for _, item := range c.items {
		if !fn(item) {
			return false
		}
	}
	return true
}

// Take returns the first n items (or the last -n when n is negative).
func (c *Collection[T]) Take(n int) *Collection[T] {
	if n < 0 {
		if -n >= len(c.items) {
			return FromSlice(append([]T(nil), c.items...))
		}
		return FromSlice(append([]T(nil), c.items[len(c.items)+n:]...))
	}
	if n > len(c.items) {
		n = len(c.items)
	}
	return FromSlice(append([]T(nil), c.items[:n]...))
}

// Skip returns the collection without its first n items; n <= 0 skips
// nothing.
func (c *Collection[T]) Skip(n int) *Collection[T] {
	if n <= 0 {
		return FromSlice(append([]T(nil), c.items...))
	}
	if n >= len(c.items) {
		return FromSlice([]T{})
	}
	return FromSlice(append([]T(nil), c.items[n:]...))
}

// Chunk splits the collection into groups of the given size.
func (c *Collection[T]) Chunk(size int) []*Collection[T] {
	if size < 1 {
		return nil
	}
	var chunks []*Collection[T]
	for i := 0; i < len(c.items); i += size {
		end := i + size
		if end > len(c.items) {
			end = len(c.items)
		}
		chunks = append(chunks, FromSlice(append([]T(nil), c.items[i:end]...)))
	}
	return chunks
}

// Reverse returns the collection in reverse order.
func (c *Collection[T]) Reverse() *Collection[T] {
	out := make([]T, len(c.items))
	for i, item := range c.items {
		out[len(c.items)-1-i] = item
	}
	return FromSlice(out)
}

// SortBy sorts using the given less function (stable).
func (c *Collection[T]) SortBy(less func(a, b T) bool) *Collection[T] {
	out := append([]T(nil), c.items...)
	sort.SliceStable(out, func(i, j int) bool { return less(out[i], out[j]) })
	return FromSlice(out)
}

// Push appends items to the collection in place and returns it.
func (c *Collection[T]) Push(items ...T) *Collection[T] {
	c.items = append(c.items, items...)
	return c
}

// Concat returns a new collection with the other collection's items appended.
func (c *Collection[T]) Concat(other *Collection[T]) *Collection[T] {
	out := append(append([]T(nil), c.items...), other.items...)
	return FromSlice(out)
}

// Tap calls fn with the collection and returns it, for side effects mid-chain.
func (c *Collection[T]) Tap(fn func(*Collection[T])) *Collection[T] {
	fn(c)
	return c
}

// Map transforms a collection into one of a different element type.
func Map[T, R any](c *Collection[T], fn func(T) R) *Collection[R] {
	out := make([]R, c.Len())
	for i, item := range c.All() {
		out[i] = fn(item)
	}
	return FromSlice(out)
}

// Reduce folds the collection into a single value.
func Reduce[T, R any](c *Collection[T], initial R, fn func(carry R, item T) R) R {
	carry := initial
	for _, item := range c.All() {
		carry = fn(carry, item)
	}
	return carry
}

// Pluck extracts one value per item.
func Pluck[T, R any](c *Collection[T], fn func(T) R) []R {
	return Map(c, fn).All()
}

// GroupBy groups items by the key fn returns, preserving item order per group.
func GroupBy[T any, K comparable](c *Collection[T], fn func(T) K) map[K][]T {
	out := make(map[K][]T)
	for _, item := range c.All() {
		key := fn(item)
		out[key] = append(out[key], item)
	}
	return out
}

// KeyBy indexes items by the key fn returns; later items overwrite earlier ones.
func KeyBy[T any, K comparable](c *Collection[T], fn func(T) K) map[K]T {
	out := make(map[K]T)
	for _, item := range c.All() {
		out[fn(item)] = item
	}
	return out
}

// Unique removes duplicates using the key fn returns, keeping first occurrences.
func Unique[T any, K comparable](c *Collection[T], fn func(T) K) *Collection[T] {
	seen := make(map[K]struct{}, c.Len())
	out := make([]T, 0, c.Len())
	for _, item := range c.All() {
		key := fn(item)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	return FromSlice(out)
}

// Sum adds up one numeric value per item.
func Sum[T any, N int | int64 | float64](c *Collection[T], fn func(T) N) N {
	var total N
	for _, item := range c.All() {
		total += fn(item)
	}
	return total
}
