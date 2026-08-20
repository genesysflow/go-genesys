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
	sequence   int
}

// NewFactory creates a factory from a definition; the definition receives
// a monotonically increasing sequence number for uniqueness.
func NewFactory[T any](definition func(i int) *T) *Factory[T] {
	return &Factory[T]{definition: definition}
}

// Make builds count instances without persisting them.
func (f *Factory[T]) Make(count int, overrides ...func(*T)) []*T {
	out := make([]*T, count)
	for i := 0; i < count; i++ {
		f.sequence++
		model := f.definition(f.sequence)
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
