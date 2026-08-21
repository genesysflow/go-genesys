package database_test

import (
	"fmt"
	"testing"

	"github.com/genesysflow/go-genesys/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFactoryMakeAndCreate(t *testing.T) {
	setupORM(t)

	factory := database.NewFactory(func(i int) *User {
		return &User{
			Name:   fmt.Sprintf("User %d", i),
			Email:  fmt.Sprintf("user%d@example.com", i),
			Age:    20 + i,
			Active: true,
		}
	})

	// Make does not persist.
	made := factory.Make(2)
	require.Len(t, made, 2)
	assert.Equal(t, "User 1", made[0].Name)
	assert.Equal(t, "User 2", made[1].Name)
	count, err := database.Query[User]().Count()
	require.NoError(t, err)
	assert.Zero(t, count)

	// Create persists with sequential values continuing the sequence.
	created, err := factory.Create(3)
	require.NoError(t, err)
	assert.EqualValues(t, 1, created[0].ID)
	assert.Equal(t, "User 3", created[0].Name)

	count, err = database.Query[User]().Count()
	require.NoError(t, err)
	assert.EqualValues(t, 3, count)

	// Overrides apply before persisting.
	admin, err := factory.CreateOne(func(u *User) { u.Name = "Admin" })
	require.NoError(t, err)
	assert.Equal(t, "Admin", admin.Name)
	found, err := database.Find[User](admin.ID)
	require.NoError(t, err)
	assert.Equal(t, "Admin", found.Name)
}
