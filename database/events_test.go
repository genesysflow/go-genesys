package database_test

import (
	"errors"
	"testing"

	"github.com/genesysflow/go-genesys/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

// Track hooks fire order on the model itself.
type AuditedNote struct {
	database.Model
	Body  string `db:"body"`
	Slug  string `db:"slug"`
	trail []string
}

func (n *AuditedNote) TableName() string { return "notes" }
func (n *AuditedNote) Saving() error     { n.trail = append(n.trail, "saving"); return nil }
func (n *AuditedNote) Saved() error      { n.trail = append(n.trail, "saved"); return nil }
func (n *AuditedNote) Creating() error {
	n.trail = append(n.trail, "creating")
	n.Slug = "slug-" + n.Body // hooks may mutate the model before insert
	return nil
}
func (n *AuditedNote) Created() error  { n.trail = append(n.trail, "created"); return nil }
func (n *AuditedNote) Updating() error { n.trail = append(n.trail, "updating"); return nil }
func (n *AuditedNote) Updated() error  { n.trail = append(n.trail, "updated"); return nil }
func (n *AuditedNote) Deleting() error { n.trail = append(n.trail, "deleting"); return nil }
func (n *AuditedNote) Deleted() error  { n.trail = append(n.trail, "deleted"); return nil }

type VetoedNote struct {
	database.Model
	Body string `db:"body"`
}

func (n *VetoedNote) TableName() string { return "notes" }
func (n *VetoedNote) Creating() error   { return errors.New("vetoed") }

func setupNotes(t *testing.T) {
	t.Helper()
	manager := database.NewManager(database.Config{
		Default: "test",
		Connections: map[string]database.ConnectionConfig{
			"test": {Driver: "sqlite", Database: ":memory:"},
		},
	})
	t.Cleanup(func() {
		database.SetDefault(nil)
		database.ClearObservers()
		manager.Close()
	})
	database.SetDefault(manager)

	_, err := manager.Statement(`CREATE TABLE notes (
		id INTEGER PRIMARY KEY AUTOINCREMENT, body TEXT, slug TEXT DEFAULT '',
		created_at TIMESTAMP, updated_at TIMESTAMP)`)
	require.NoError(t, err)
}

func TestModelHooksFireInOrder(t *testing.T) {
	setupNotes(t)

	note := &AuditedNote{Body: "hello"}
	require.NoError(t, database.Create(note))
	assert.Equal(t, []string{"saving", "creating", "created", "saved"}, note.trail)
	assert.Equal(t, "slug-hello", note.Slug, "Creating hook mutation persisted")

	stored, err := database.Find[AuditedNote](note.ID)
	require.NoError(t, err)
	assert.Equal(t, "slug-hello", stored.Slug)

	note.trail = nil
	note.Body = "changed"
	require.NoError(t, database.Update(note))
	assert.Equal(t, []string{"saving", "updating", "updated", "saved"}, note.trail)

	note.trail = nil
	require.NoError(t, database.DeleteModel(note))
	assert.Equal(t, []string{"deleting", "deleted"}, note.trail)
}

func TestHookErrorAbortsOperation(t *testing.T) {
	setupNotes(t)

	err := database.Create(&VetoedNote{Body: "never"})
	assert.ErrorContains(t, err, "vetoed")

	count, err := database.Default().Table("notes").Count()
	require.NoError(t, err)
	assert.EqualValues(t, 0, count, "vetoed insert never reached the database")
}

func TestObservers(t *testing.T) {
	setupNotes(t)

	var events []string
	database.Observe(database.Observer[AuditedNote]{
		Created: func(n *AuditedNote) error { events = append(events, "obs:created:"+n.Body); return nil },
		Updated: func(n *AuditedNote) error { events = append(events, "obs:updated"); return nil },
		Deleted: func(n *AuditedNote) error { events = append(events, "obs:deleted"); return nil },
	})

	note := &AuditedNote{Body: "watched"}
	require.NoError(t, database.Create(note))
	note.Body = "watched2"
	require.NoError(t, database.Update(note))
	require.NoError(t, database.DeleteModel(note))

	assert.Equal(t, []string{"obs:created:watched", "obs:updated", "obs:deleted"}, events)
}

func TestObserverErrorAborts(t *testing.T) {
	setupNotes(t)

	database.Observe(database.Observer[AuditedNote]{
		Creating: func(n *AuditedNote) error { return errors.New("observer says no") },
	})

	err := database.Create(&AuditedNote{Body: "no"})
	assert.ErrorContains(t, err, "observer says no")
}

// --- dirty tracking ---

func TestDirtyTracking(t *testing.T) {
	setupORM(t)
	user := &User{Name: "Ada", Email: "ada@x.io", Age: 30, Active: true}

	// Unsaved models are entirely dirty.
	assert.True(t, database.IsDirty(user))
	assert.Nil(t, database.GetOriginal(user))

	require.NoError(t, database.Create(user))
	assert.False(t, database.IsDirty(user), "clean right after create")
	require.NotNil(t, database.GetOriginal(user))

	user.Name = "Ada Lovelace"
	assert.True(t, database.IsDirty(user))
	assert.True(t, database.IsDirty(user, "name"))
	assert.False(t, database.IsDirty(user, "email"))
	dirty := database.GetDirty(user)
	assert.Equal(t, map[string]any{"name": "Ada Lovelace"}, dirty)

	require.NoError(t, database.Update(user))
	assert.False(t, database.IsDirty(user), "clean after update")

	// Fetched models are clean and track from their fetched state.
	fetched, err := database.Find[User](user.ID)
	require.NoError(t, err)
	assert.False(t, database.IsDirty(fetched))
	fetched.Age = 31
	assert.Equal(t, map[string]any{"age": 31}, database.GetDirty(fetched))
}

func TestCleanUpdateSkipsQuery(t *testing.T) {
	setupORM(t)
	user := &User{Name: "Quiet", Email: "q@x.io"}
	require.NoError(t, database.Create(user))

	before, err := database.Find[User](user.ID)
	require.NoError(t, err)

	// Updating a clean model touches nothing - not even updated_at.
	require.NoError(t, database.Update(user))
	after, err := database.Find[User](user.ID)
	require.NoError(t, err)
	assert.Equal(t, before.UpdatedAt, after.UpdatedAt)
}

func TestPartialUpdateOnlyWritesDirtyColumns(t *testing.T) {
	setupORM(t)
	user := &User{Name: "Partial", Email: "p@x.io", Age: 20}
	require.NoError(t, database.Create(user))

	// Sneak a change into the row behind the model's back.
	_, err := database.Default().Table("users").Where("id", user.ID).Update(map[string]any{"age": 99})
	require.NoError(t, err)

	// The model only knows name changed, so age must survive the update.
	user.Name = "Partial2"
	require.NoError(t, database.Update(user))

	fetched, err := database.Find[User](user.ID)
	require.NoError(t, err)
	assert.Equal(t, "Partial2", fetched.Name)
	assert.Equal(t, 99, fetched.Age, "undirty column not clobbered")
}
