package testutil_test

import (
	"testing"

	"github.com/genesysflow/go-genesys/database"
	"github.com/genesysflow/go-genesys/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

type Member struct {
	database.Model
	Name  string `db:"name"`
	Email string `db:"email"`
}

func setupMembers(t *testing.T) {
	t.Helper()

	manager := database.NewManager(database.Config{
		Default: "test",
		Connections: map[string]database.ConnectionConfig{
			"test": {Driver: "sqlite", Database: ":memory:"},
		},
	})
	t.Cleanup(func() {
		database.SetDefault(nil)
		_ = manager.Close()
	})
	database.SetDefault(manager)

	_, err := manager.Statement(`CREATE TABLE members (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		email TEXT NOT NULL,
		created_at TIMESTAMP,
		updated_at TIMESTAMP
	)`)
	require.NoError(t, err)
}

func TestAssertDatabaseHas(t *testing.T) {
	setupMembers(t)
	require.NoError(t, database.Create(&Member{Name: "Ada", Email: "ada@example.com"}))

	testutil.AssertDatabaseHas(t, "members", map[string]any{"email": "ada@example.com"})
	testutil.AssertDatabaseMissing(t, "members", map[string]any{"email": "nobody@example.com"})
}

func TestAssertDatabaseCount(t *testing.T) {
	setupMembers(t)
	require.NoError(t, database.Create(&Member{Name: "Ada", Email: "ada@example.com"}))
	require.NoError(t, database.Create(&Member{Name: "Grace", Email: "grace@example.com"}))

	testutil.AssertDatabaseCount(t, "members", 2)
}

// The assertions must fail when the row is not there - an assertion that
// cannot fail proves nothing.
func TestDatabaseAssertionsFailLoudly(t *testing.T) {
	setupMembers(t)

	spy := &recordingT{}
	testutil.AssertDatabaseHas(spy, "members", map[string]any{"email": "ada@example.com"})
	assert.NotEmpty(t, spy.failures, "a missing row should fail the assertion")

	require.NoError(t, database.Create(&Member{Name: "Ada", Email: "ada@example.com"}))

	spy = &recordingT{}
	testutil.AssertDatabaseMissing(spy, "members", map[string]any{"email": "ada@example.com"})
	assert.NotEmpty(t, spy.failures, "a present row should fail the missing assertion")

	spy = &recordingT{}
	testutil.AssertDatabaseCount(spy, "members", 5)
	assert.NotEmpty(t, spy.failures)
}

// A table that does not exist is a test-setup mistake, and must be
// reported rather than read as "no matching row".
func TestAssertDatabaseHasUnknownTable(t *testing.T) {
	setupMembers(t)

	spy := &recordingT{}
	testutil.AssertDatabaseHas(spy, "nope", map[string]any{"id": 1})
	assert.NotEmpty(t, spy.failures)
}

// --- soft deletes ----------------------------------------------------

type Draft struct {
	database.Model
	Title     string  `db:"title"`
	DeletedAt *string `db:"deleted_at"`
}

func TestAssertSoftDeleted(t *testing.T) {
	setupMembers(t)

	manager := database.Default()
	_, err := manager.Statement(`CREATE TABLE drafts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		title TEXT NOT NULL,
		deleted_at TIMESTAMP,
		created_at TIMESTAMP,
		updated_at TIMESTAMP
	)`)
	require.NoError(t, err)

	draft := &Draft{Title: "Notes"}
	require.NoError(t, database.Create(draft))

	testutil.AssertNotSoftDeleted(t, "drafts", map[string]any{"id": draft.ID})

	_, err = database.Query[Draft]().Where("id", draft.ID).Delete()
	require.NoError(t, err)

	testutil.AssertSoftDeleted(t, "drafts", map[string]any{"id": draft.ID})
}

// recordingT captures assertion failures instead of failing the test.
type recordingT struct{ failures []string }

func (r *recordingT) Helper() {}

func (r *recordingT) Errorf(format string, args ...any) {
	r.failures = append(r.failures, format)
}
