package support

import "sort"

// MessageBag holds messages keyed by field, Laravel's `$errors` bag. It
// is what validation failures are flashed as and what views receive, so
// templates can ask `{{if .errors.Has "email"}}` without knowing where
// the messages came from.
type MessageBag struct {
	messages map[string][]string
}

// NewMessageBag builds a bag from field messages. The source map is
// copied, so later mutations of it do not reach into the bag.
func NewMessageBag(messages map[string][]string) *MessageBag {
	bag := &MessageBag{messages: make(map[string][]string, len(messages))}
	for field, list := range messages {
		bag.messages[field] = append([]string(nil), list...)
	}
	return bag
}

// Add appends a message for a field.
func (b *MessageBag) Add(field, message string) *MessageBag {
	if b.messages == nil {
		b.messages = make(map[string][]string)
	}
	b.messages[field] = append(b.messages[field], message)
	return b
}

// Has reports whether the bag holds any message for the field.
func (b *MessageBag) Has(field string) bool {
	return len(b.messages[field]) > 0
}

// Any reports whether the bag holds any message at all.
func (b *MessageBag) Any() bool {
	return b.Count() > 0
}

// Count returns the total number of messages across all fields.
func (b *MessageBag) Count() int {
	total := 0
	for _, list := range b.messages {
		total += len(list)
	}
	return total
}

// Get returns every message for a field.
func (b *MessageBag) Get(field string) []string {
	return append([]string(nil), b.messages[field]...)
}

// First returns the first message for the given field, or - when called
// without a field - the first message in the bag. Fields are visited in
// sorted order so the answer is stable.
func (b *MessageBag) First(field ...string) string {
	if len(field) > 0 {
		if list := b.messages[field[0]]; len(list) > 0 {
			return list[0]
		}
		return ""
	}

	for _, key := range b.Keys() {
		if list := b.messages[key]; len(list) > 0 {
			return list[0]
		}
	}
	return ""
}

// All returns every message in the bag, flattened, ordered by field.
func (b *MessageBag) All() []string {
	all := make([]string, 0, b.Count())
	for _, key := range b.Keys() {
		all = append(all, b.messages[key]...)
	}
	return all
}

// Keys returns the fields holding messages, sorted.
func (b *MessageBag) Keys() []string {
	keys := make([]string, 0, len(b.messages))
	for key := range b.messages {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// Messages returns a copy of the messages keyed by field.
func (b *MessageBag) Messages() map[string][]string {
	out := make(map[string][]string, len(b.messages))
	for field, list := range b.messages {
		out[field] = append([]string(nil), list...)
	}
	return out
}

// IsEmpty reports whether the bag holds no messages.
func (b *MessageBag) IsEmpty() bool {
	return b.Count() == 0
}
