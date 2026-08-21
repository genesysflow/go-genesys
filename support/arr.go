package support

import "maps"

// MapSlice transforms every element. It is named MapSlice rather than
// Map because Map reads as the container in Go.
func MapSlice[T, R any](items []T, fn func(T) R) []R {
	out := make([]R, 0, len(items))
	for _, item := range items {
		out = append(out, fn(item))
	}
	return out
}

// Filter keeps the elements the predicate accepts.
func Filter[T any](items []T, keep func(T) bool) []T {
	out := make([]T, 0, len(items))
	for _, item := range items {
		if keep(item) {
			out = append(out, item)
		}
	}
	return out
}

// Reject is the inverse of Filter.
func Reject[T any](items []T, drop func(T) bool) []T {
	return Filter(items, func(item T) bool { return !drop(item) })
}

// Reduce folds the elements into a single value.
func Reduce[T, R any](items []T, initial R, fn func(carry R, item T) R) R {
	carry := initial
	for _, item := range items {
		carry = fn(carry, item)
	}
	return carry
}

// Unique removes duplicates, keeping the first occurrence of each.
func Unique[T comparable](items []T) []T {
	seen := make(map[T]struct{}, len(items))
	out := make([]T, 0, len(items))
	for _, item := range items {
		if _, found := seen[item]; found {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

// UniqueBy removes duplicates by a derived key.
func UniqueBy[T any, K comparable](items []T, key func(T) K) []T {
	seen := make(map[K]struct{}, len(items))
	out := make([]T, 0, len(items))
	for _, item := range items {
		k := key(item)
		if _, found := seen[k]; found {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, item)
	}
	return out
}

// Flatten concatenates a slice of slices.
func Flatten[T any](groups [][]T) []T {
	out := make([]T, 0)
	for _, group := range groups {
		out = append(out, group...)
	}
	return out
}

// Chunk splits a slice into runs of at most size. A size of zero yields
// nothing rather than looping forever.
func Chunk[T any](items []T, size int) [][]T {
	if size <= 0 {
		return nil
	}

	chunks := make([][]T, 0, (len(items)+size-1)/size)
	for start := 0; start < len(items); start += size {
		end := start + size
		if end > len(items) {
			end = len(items)
		}
		chunks = append(chunks, items[start:end])
	}
	return chunks
}

// KeyBy indexes the elements by a derived key; later duplicates win.
func KeyBy[T any, K comparable](items []T, key func(T) K) map[K]T {
	out := make(map[K]T, len(items))
	for _, item := range items {
		out[key(item)] = item
	}
	return out
}

// GroupBy collects the elements under a derived key.
func GroupBy[T any, K comparable](items []T, key func(T) K) map[K][]T {
	out := make(map[K][]T)
	for _, item := range items {
		k := key(item)
		out[k] = append(out[k], item)
	}
	return out
}

// Pluck extracts one value from each element.
func Pluck[T any, R any](items []T, value func(T) R) []R {
	return MapSlice(items, value)
}

// Only returns a copy of the map holding just the named keys, which is
// how a payload is narrowed before it is stored or logged.
func Only[K comparable, V any](source map[K]V, keys ...K) map[K]V {
	out := make(map[K]V, len(keys))
	for _, key := range keys {
		if value, found := source[key]; found {
			out[key] = value
		}
	}
	return out
}

// Except returns a copy of the map without the named keys - dropping a
// password before something is logged, for instance. The source is left
// alone.
func Except[K comparable, V any](source map[K]V, keys ...K) map[K]V {
	out := make(map[K]V, len(source))
	maps.Copy(out, source)
	for _, key := range keys {
		delete(out, key)
	}
	return out
}

// First returns the first element matching the predicate.
func First[T any](items []T, match func(T) bool) (T, bool) {
	for _, item := range items {
		if match(item) {
			return item, true
		}
	}
	var zero T
	return zero, false
}

// Last returns the last element matching the predicate.
func Last[T any](items []T, match func(T) bool) (T, bool) {
	for i := len(items) - 1; i >= 0; i-- {
		if match(items[i]) {
			return items[i], true
		}
	}
	var zero T
	return zero, false
}

// Partition splits the elements into those matching the predicate and
// the rest, in one pass.
func Partition[T any](items []T, match func(T) bool) (matching, rest []T) {
	matching = make([]T, 0, len(items))
	rest = make([]T, 0, len(items))
	for _, item := range items {
		if match(item) {
			matching = append(matching, item)
			continue
		}
		rest = append(rest, item)
	}
	return matching, rest
}

// Includes reports whether the slice holds the value.
func Includes[T comparable](items []T, value T) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}
