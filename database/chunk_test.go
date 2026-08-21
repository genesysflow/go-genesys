package database_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/genesysflow/go-genesys/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func seedManyUsers(t *testing.T, n int) {
	t.Helper()
	setupORM(t)
	for i := 1; i <= n; i++ {
		require.NoError(t, database.Create(&User{
			Name:  fmt.Sprintf("User %02d", i),
			Email: fmt.Sprintf("u%d@x.io", i),
			Age:   20 + i%10,
		}))
	}
}

func TestCursorPaginate(t *testing.T) {
	seedManyUsers(t, 7)

	var all []string
	cursor := ""
	pages := 0
	for {
		page, err := database.Query[User]().CursorPaginate(3, cursor)
		require.NoError(t, err)
		pages++
		for _, u := range page.Data {
			all = append(all, u.Name)
		}
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}

	assert.Equal(t, 3, pages, "7 rows in pages of 3")
	assert.Len(t, all, 7)
	assert.Equal(t, "User 01", all[0])
	assert.Equal(t, "User 07", all[6])
}

func TestCursorPaginateStableUnderInserts(t *testing.T) {
	seedManyUsers(t, 4)

	page, err := database.Query[User]().CursorPaginate(2, "")
	require.NoError(t, err)
	require.Len(t, page.Data, 2)

	// Rows inserted after the cursor position don't shift the window.
	require.NoError(t, database.Create(&User{Name: "User 99", Email: "l@x.io"}))

	page2, err := database.Query[User]().CursorPaginate(2, page.NextCursor)
	require.NoError(t, err)
	assert.Equal(t, "User 03", page2.Data[0].Name)
	assert.Equal(t, "User 04", page2.Data[1].Name)

	// Bad cursors error cleanly.
	_, err = database.Query[User]().CursorPaginate(2, "!!!not-base64!!!")
	assert.ErrorContains(t, err, "malformed cursor")
}

func TestCursorPaginateWithFiltersAndSoftDeletes(t *testing.T) {
	_, trashed := setupDocuments(t) // 1 live, 1 trashed

	page, err := database.Query[Document]().CursorPaginate(10, "")
	require.NoError(t, err)
	require.Len(t, page.Data, 1, "trashed rows excluded")
	assert.Equal(t, "kept", page.Data[0].Title)

	page, err = database.Query[Document]().WithTrashed().CursorPaginate(10, "")
	require.NoError(t, err)
	assert.Len(t, page.Data, 2)
	_ = trashed
}

func TestChunkWalksEverythingInOrder(t *testing.T) {
	seedManyUsers(t, 10)

	var batches [][]string
	err := database.Query[User]().Chunk(4, func(users []User) error {
		names := make([]string, len(users))
		for i, u := range users {
			names[i] = u.Name
		}
		batches = append(batches, names)
		return nil
	})
	require.NoError(t, err)

	require.Len(t, batches, 3, "10 rows in chunks of 4 -> 4,4,2")
	assert.Len(t, batches[0], 4)
	assert.Len(t, batches[2], 2)
	assert.Equal(t, "User 01", batches[0][0])
	assert.Equal(t, "User 10", batches[2][1])
}

func TestChunkIsDeleteSafe(t *testing.T) {
	seedManyUsers(t, 6)

	// Deleting each processed row must not skip any others (the classic
	// offset-chunk bug).
	var seen []string
	err := database.Query[User]().Chunk(2, func(users []User) error {
		for _, u := range users {
			seen = append(seen, u.Name)
			if err := database.Delete[User](u.ID); err != nil {
				return err
			}
		}
		return nil
	})
	require.NoError(t, err)
	assert.Len(t, seen, 6, "every row visited exactly once")

	count, err := database.Query[User]().Count()
	require.NoError(t, err)
	assert.Zero(t, count)
}

func TestChunkRespectsFiltersAndStopsOnError(t *testing.T) {
	seedManyUsers(t, 6)

	// Filters carry into every chunk query.
	var seen int
	err := database.Query[User]().Where("age", ">", 24).Chunk(2, func(users []User) error {
		seen += len(users)
		return nil
	})
	require.NoError(t, err)
	expected, err := database.Query[User]().Where("age", ">", 24).Count()
	require.NoError(t, err)
	assert.EqualValues(t, expected, seen)

	// A callback error stops iteration and propagates.
	calls := 0
	err = database.Query[User]().Chunk(2, func(users []User) error {
		calls++
		return errors.New("stop here")
	})
	assert.ErrorContains(t, err, "stop here")
	assert.Equal(t, 1, calls)
}

func TestEach(t *testing.T) {
	seedManyUsers(t, 5)

	var names []string
	err := database.Query[User]().Each(func(u *User) error {
		names = append(names, u.Name)
		return nil
	})
	require.NoError(t, err)
	assert.Len(t, names, 5)
	assert.Equal(t, "User 01", names[0])
}
