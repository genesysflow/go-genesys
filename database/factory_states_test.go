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
