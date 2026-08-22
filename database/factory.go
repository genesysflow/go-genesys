package database

// Factory builds and persists model instances for seeding and tests,
// mirroring Laravel model factories:
//
//	userFactory := database.NewFactory(func(i int) *User {
//	    return &User{
//	        Name:  fmt.Sprintf("User %d", i),
//	        Email: fmt.Sprintf("user%d@example.com", i),
//	    }
//	})
//
//	users, err := userFactory.Create(10)
//	admin, err := userFactory.CreateOne(func(u *User) { u.Admin = true })
type Factory[T any] struct {
	definition func(i int) *T
	states     []func(*T)
	sequences  []func(*T)
	sequence   int
}

// NewFactory creates a factory from a definition; the definition receives
// a monotonically increasing sequence number for uniqueness.
func NewFactory[T any](definition func(i int) *T) *Factory[T] {
	return &Factory[T]{definition: definition}
}

// State returns a factory that applies one more variation on top of this
// one, Laravel's factory states:
//
//	inactiveUsers := userFactory.State(func(u *User) { u.Active = false })
//
// The receiver is left alone, so states can be layered without one test's
// variation leaking into another's.
func (f *Factory[T]) State(state func(*T)) *Factory[T] {
	clone := f.clone()
	clone.states = append(clone.states, state)
	return clone
}

// Sequence returns a factory that cycles the given variations across the
// models it makes, so a test gets a spread of rows without a loop:
//
//	userFactory.Sequence(
//	    func(u *User) { u.Plan = "free" },
//	    func(u *User) { u.Plan = "pro" },
//	).Create(10)
func (f *Factory[T]) Sequence(states ...func(*T)) *Factory[T] {
	clone := f.clone()
	clone.sequences = append(clone.sequences, states...)
	return clone
}

// clone copies the factory's configuration, sharing nothing mutable.
func (f *Factory[T]) clone() *Factory[T] {
	states := make([]func(*T), len(f.states))
	copy(states, f.states)

	sequences := make([]func(*T), len(f.sequences))
	copy(sequences, f.sequences)

	return &Factory[T]{
		definition: f.definition,
		states:     states,
		sequences:  sequences,
		sequence:   f.sequence,
	}
}

// Make builds count instances without persisting them. States apply
// first, then the sequence, then the call site's overrides - the most
// specific statement wins.
func (f *Factory[T]) Make(count int, overrides ...func(*T)) []*T {
	out := make([]*T, count)
	for i := 0; i < count; i++ {
		f.sequence++
		model := f.definition(f.sequence)

		for _, state := range f.states {
			state(model)
		}
		if len(f.sequences) > 0 {
			// Indexed on the factory's own counter, not the loop's: a
			// sequence cycles across everything the factory makes, so
			// calling MakeOne twice gives the first state then the
			// second rather than the first twice.
			f.sequences[(f.sequence-1)%len(f.sequences)](model)
		}
		for _, override := range overrides {
			override(model)
		}

		out[i] = model
	}
	return out
}

// MakeOne builds a single instance without persisting it.
func (f *Factory[T]) MakeOne(overrides ...func(*T)) *T {
	return f.Make(1, overrides...)[0]
}

// Create builds and persists count instances.
func (f *Factory[T]) Create(count int, overrides ...func(*T)) ([]*T, error) {
	models := f.Make(count, overrides...)
	for _, model := range models {
		if err := Create(model); err != nil {
			return nil, err
		}
	}
	return models, nil
}

// CreateOne builds and persists a single instance.
func (f *Factory[T]) CreateOne(overrides ...func(*T)) (*T, error) {
	models, err := f.Create(1, overrides...)
	if err != nil {
		return nil, err
	}
	return models[0], nil
}
