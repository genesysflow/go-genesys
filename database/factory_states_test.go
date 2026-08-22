package database_test

import (
	"fmt"
	"testing"

	"github.com/genesysflow/go-genesys/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func userFactory() *database.Factory[User] {
	return database.NewFactory(func(i int) *User {
		return &User{
			Name:   fmt.Sprintf("User %d", i),
			Email:  fmt.Sprintf("user%d@example.com", i),
			Age:    30,
			Active: true,
		}
	})
}

// A state is a named variation of the factory, so tests read as
// "an inactive user" rather than a pile of overrides.
func TestFactoryState(t *testing.T) {
	inactive := userFactory().State(func(u *User) { u.Active = false })

	users := inactive.Make(2)
	require.Len(t, users, 2)
	for _, user := range users {
		assert.False(t, user.Active)
		assert.NotEmpty(t, user.Name, "the base definition still applies")
	}
}

// States compose, in the order they were applied.
func TestFactoryStatesCompose(t *testing.T) {
	factory := userFactory().
		State(func(u *User) { u.Age = 18 }).
		State(func(u *User) { u.Age = 21 })

	assert.Equal(t, 21, factory.MakeOne().Age)
}

// A state must not leak back into the factory it came from.
func TestFactoryStateDoesNotMutateTheOriginal(t *testing.T) {
	base := userFactory()
	inactive := base.State(func(u *User) { u.Active = false })

	assert.True(t, base.MakeOne().Active)
	assert.False(t, inactive.MakeOne().Active)
}

// A sequence cycles values across the models it makes, which is how a
// test builds a spread of rows without writing a loop.
func TestFactorySequence(t *testing.T) {
	factory := userFactory().Sequence(
		func(u *User) { u.Age = 20 },
		func(u *User) { u.Age = 30 },
	)

	users := factory.Make(4)
	require.Len(t, users, 4)
	assert.Equal(t, []int{20, 30, 20, 30}, []int{users[0].Age, users[1].Age, users[2].Age, users[3].Age})
}

func TestFactoryCreateAppliesStates(t *testing.T) {
	setupORM(t)

	created, err := userFactory().State(func(u *User) { u.Active = false }).Create(2)
	require.NoError(t, err)
	require.Len(t, created, 2)

	stored, err := database.Query[User]().Where("active", false).Get()
	require.NoError(t, err)
	assert.Len(t, stored, 2)
}

// Overrides passed at the call site win over states.
func TestFactoryOverridesBeatStates(t *testing.T) {
	factory := userFactory().State(func(u *User) { u.Age = 18 })

	user := factory.MakeOne(func(u *User) { u.Age = 99 })
	assert.Equal(t, 99, user.Age)
}

// A sequence cycles across everything the factory makes, not within one
// call. Indexing by the per-call loop counter means CreateOne() always
// applies the first state, which is exactly the case a sequence is most
// useful for: alternating rows as a test creates them one at a time.
func TestSequenceCyclesAcrossCalls(t *testing.T) {
	factory := database.NewFactory(func(i int) *sequenced {
		return &sequenced{Label: "base"}
	}).Sequence(
		func(s *sequenced) { s.Label = "first" },
		func(s *sequenced) { s.Label = "second" },
	)

	// One at a time is the common case.
	assert.Equal(t, "first", factory.MakeOne().Label)
	assert.Equal(t, "second", factory.MakeOne().Label)
	assert.Equal(t, "first", factory.MakeOne().Label, "the sequence should wrap")
	assert.Equal(t, "second", factory.MakeOne().Label)
}

// A batch keeps cycling from where the factory left off rather than
// restarting.
func TestSequenceContinuesIntoABatch(t *testing.T) {
	factory := database.NewFactory(func(i int) *sequenced {
		return &sequenced{Label: "base"}
	}).Sequence(
		func(s *sequenced) { s.Label = "a" },
		func(s *sequenced) { s.Label = "b" },
	)

	assert.Equal(t, "a", factory.MakeOne().Label)

	batch := factory.Make(3)
	assert.Equal(t, []string{"b", "a", "b"},
		[]string{batch[0].Label, batch[1].Label, batch[2].Label})
}

type sequenced struct {
	database.Model
	Label string `db:"label"`
}
