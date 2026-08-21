package database_test

import (
	"testing"

	"github.com/genesysflow/go-genesys/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

// --- Fresh / Refresh -------------------------------------------------

func TestFreshReadsTheStoredRow(t *testing.T) {
	setupORM(t)

	user := &User{Name: "Ada", Email: "ada@example.com"}
	require.NoError(t, database.Create(user))

	// Someone else updates the row.
	_, err := database.Query[User]().Where("id", user.ID).Update(map[string]any{"name": "Ada L."})
	require.NoError(t, err)

	fresh, err := database.Fresh(user)
	require.NoError(t, err)
	assert.Equal(t, "Ada L.", fresh.Name)
	assert.Equal(t, "Ada", user.Name, "Fresh returns a new instance and leaves the original alone")
}

func TestRefreshUpdatesInPlace(t *testing.T) {
	setupORM(t)

	user := &User{Name: "Ada", Email: "ada@example.com"}
	require.NoError(t, database.Create(user))

	_, err := database.Query[User]().Where("id", user.ID).Update(map[string]any{"name": "Ada L."})
	require.NoError(t, err)

	require.NoError(t, database.Refresh(user))
	assert.Equal(t, "Ada L.", user.Name)
}

// Refreshing a row that has since been deleted is an error, not a
// silently stale model.
func TestRefreshDeletedRow(t *testing.T) {
	setupORM(t)

	user := &User{Name: "Ada", Email: "ada@example.com"}
	require.NoError(t, database.Create(user))
	require.NoError(t, database.Delete[User](user.ID))

	assert.Error(t, database.Refresh(user))
}

// --- Replicate -------------------------------------------------------

func TestReplicate(t *testing.T) {
	setupORM(t)

	user := &User{Name: "Ada", Email: "ada@example.com", Age: 36}
	require.NoError(t, database.Create(user))

	copy := database.Replicate(user)
	assert.Equal(t, int64(0), copy.ID, "a replica is unsaved")
	assert.True(t, copy.CreatedAt.IsZero())
	assert.Equal(t, "Ada", copy.Name)

	copy.Email = "ada+copy@example.com"
	require.NoError(t, database.Create(copy))
	assert.NotEqual(t, user.ID, copy.ID)
}

func TestReplicateWithout(t *testing.T) {
	setupORM(t)

	user := &User{Name: "Ada", Email: "ada@example.com", Age: 36}
	require.NoError(t, database.Create(user))

	copy := database.Replicate(user, "email")
	assert.Equal(t, "", copy.Email)
	assert.Equal(t, "Ada", copy.Name)
}

// --- Is --------------------------------------------------------------

func TestIs(t *testing.T) {
	setupORM(t)

	first := &User{Name: "Ada", Email: "ada@example.com"}
	second := &User{Name: "Grace", Email: "grace@example.com"}
	require.NoError(t, database.Create(first))
	require.NoError(t, database.Create(second))

	same, err := database.Find[User](first.ID)
	require.NoError(t, err)

	assert.True(t, database.Is(first, same))
	assert.False(t, database.Is(first, second))
	assert.False(t, database.Is[User](nil, first))
}

// --- ToMap / hidden --------------------------------------------------

// Account hides its password and exposes a computed field, the way a
// model that is serialized straight into a response must.
type Account struct {
	database.Model
	Email    string `db:"email" json:"email"`
	Password string `db:"password" json:"password"`
	First    string `db:"first_name" json:"first_name"`
	Last     string `db:"last_name" json:"last_name"`
}

func (a *Account) Hidden() []string { return []string{"password"} }

func (a *Account) Appends() map[string]any {
	return map[string]any{"full_name": a.First + " " + a.Last}
}

func TestToMapHidesAndAppends(t *testing.T) {
	account := &Account{
		Model:    database.Model{ID: 7},
		Email:    "ada@example.com",
		Password: "hashed",
		First:    "Ada",
		Last:     "Lovelace",
	}

	rendered := database.ToMap(account)

	assert.Equal(t, "ada@example.com", rendered["email"])
	assert.Equal(t, int64(7), rendered["id"])
	assert.Equal(t, "Ada Lovelace", rendered["full_name"])

	_, present := rendered["password"]
	assert.False(t, present, "a hidden field must not be serialized")
}

// A model with nothing hidden serializes every column.
func TestToMapWithoutHidden(t *testing.T) {
	rendered := database.ToMap(&User{Model: database.Model{ID: 1}, Name: "Ada", Email: "ada@example.com"})

	assert.Equal(t, "Ada", rendered["name"])
	assert.Equal(t, "ada@example.com", rendered["email"])
}

// Visible() is the allow-list form: anything not named is dropped.
type Ticket struct {
	database.Model
	Subject  string `db:"subject" json:"subject"`
	Internal string `db:"internal" json:"internal"`
}

func (tk *Ticket) Visible() []string { return []string{"id", "subject"} }

func TestToMapVisible(t *testing.T) {
	rendered := database.ToMap(&Ticket{Model: database.Model{ID: 3}, Subject: "Help", Internal: "notes"})

	assert.Equal(t, "Help", rendered["subject"])
	assert.Equal(t, int64(3), rendered["id"])
	_, present := rendered["internal"]
	assert.False(t, present)
}
