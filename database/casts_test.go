package database_test

import (
	"testing"

	"github.com/genesysflow/go-genesys/crypt"
	"github.com/genesysflow/go-genesys/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

type CastProfile struct {
	database.Model
	Name     string         `db:"name"`
	Settings map[string]any `db:"settings,json"`
	Links    []string       `db:"links,json"`
	SSN      string         `db:"ssn,encrypted"`
	Views    int64          `db:"views"`
}

func setupCasts(t *testing.T) {
	t.Helper()
	setupORM(t)
	_, err := database.Default().Statement(`CREATE TABLE cast_profiles (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		settings TEXT NOT NULL DEFAULT '',
		links TEXT NOT NULL DEFAULT '',
		ssn TEXT NOT NULL DEFAULT '',
		views INTEGER NOT NULL DEFAULT 0,
		created_at TIMESTAMP, updated_at TIMESTAMP
	)`)
	require.NoError(t, err)

	enc, err := crypt.New([]byte("0123456789abcdef0123456789abcdef"))
	require.NoError(t, err)
	database.SetEncrypter(enc)
	t.Cleanup(func() { database.SetEncrypter(nil) })
}

func TestJSONCastRoundTrip(t *testing.T) {
	setupCasts(t)

	p := &CastProfile{
		Name:     "Ada",
		Settings: map[string]any{"theme": "dark", "level": float64(3)},
		Links:    []string{"a.com", "b.com"},
	}
	require.NoError(t, database.Create(p))

	// The column holds a JSON string...
	rows, err := database.Default().Table("cast_profiles").Get()
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.JSONEq(t, `{"theme":"dark","level":3}`, rows[0]["settings"].(string))

	// ...and scans back into the typed field.
	loaded, err := database.Find[CastProfile](p.ID)
	require.NoError(t, err)
	assert.Equal(t, "dark", loaded.Settings["theme"])
	assert.Equal(t, []string{"a.com", "b.com"}, loaded.Links)

	// Dirty tracking sees json-cast changes; a clean reload is clean.
	assert.False(t, database.IsDirty(loaded))
	loaded.Settings["theme"] = "light"
	assert.True(t, database.IsDirty(loaded, "settings"))
	require.NoError(t, database.Update(loaded))

	reloaded, err := database.Find[CastProfile](p.ID)
	require.NoError(t, err)
	assert.Equal(t, "light", reloaded.Settings["theme"])
}

func TestEncryptedCastRoundTrip(t *testing.T) {
	setupCasts(t)

	p := &CastProfile{Name: "Bob", SSN: "123-45-6789"}
	require.NoError(t, database.Create(p))

	// At rest the column is ciphertext, not the plaintext.
	rows, err := database.Default().Table("cast_profiles").Get()
	require.NoError(t, err)
	stored := rows[0]["ssn"].(string)
	assert.NotEqual(t, "123-45-6789", stored)
	assert.NotContains(t, stored, "123-45")

	// The model reads back decrypted.
	loaded, err := database.Find[CastProfile](p.ID)
	require.NoError(t, err)
	assert.Equal(t, "123-45-6789", loaded.SSN)

	// A clean model stays clean despite nondeterministic ciphertexts.
	assert.False(t, database.IsDirty(loaded))
	loaded.SSN = "999-99-9999"
	require.NoError(t, database.Update(loaded))
	reloaded, err := database.Find[CastProfile](p.ID)
	require.NoError(t, err)
	assert.Equal(t, "999-99-9999", reloaded.SSN)
}

func TestModelIncrementDecrement(t *testing.T) {
	setupCasts(t)
	p := &CastProfile{Name: "Counter"}
	require.NoError(t, database.Create(p))

	affected, err := database.Query[CastProfile]().Where("id", p.ID).Increment("views", 5)
	require.NoError(t, err)
	assert.EqualValues(t, 1, affected)
	_, err = database.Query[CastProfile]().Where("id", p.ID).Decrement("views")
	require.NoError(t, err)

	loaded, err := database.Find[CastProfile](p.ID)
	require.NoError(t, err)
	assert.EqualValues(t, 4, loaded.Views)
}

func TestBulkUpsert(t *testing.T) {
	setupORM(t)
	_, err := database.Default().Statement(`CREATE TABLE flights (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		code TEXT NOT NULL UNIQUE,
		price INTEGER NOT NULL,
		created_at TIMESTAMP, updated_at TIMESTAMP
	)`)
	require.NoError(t, err)

	type Flight struct {
		database.Model
		Code  string `db:"code"`
		Price int64  `db:"price"`
	}

	_, err = database.Upsert[Flight]([]map[string]any{
		{"code": "BA9", "price": 240},
		{"code": "AF3", "price": 180},
	}, []string{"code"}, []string{"price"})
	require.NoError(t, err)

	// Colliding rows update in place instead of erroring.
	_, err = database.Upsert[Flight]([]map[string]any{
		{"code": "BA9", "price": 199},
		{"code": "LH7", "price": 320},
	}, []string{"code"}, []string{"price"})
	require.NoError(t, err)

	flights, err := database.Query[Flight]().OrderBy("code").Get()
	require.NoError(t, err)
	require.Len(t, flights, 3)
	assert.EqualValues(t, 199, flights[1].Price, "BA9 was updated, not duplicated")
	assert.False(t, flights[0].CreatedAt.IsZero(), "upsert stamps timestamps")
}

func TestTouch(t *testing.T) {
	setupORM(t)
	u := &User{Name: "T", Email: "t@x.io"}
	require.NoError(t, database.Create(u))
	before := u.UpdatedAt

	// Force a visibly different timestamp.
	_, err := database.Default().Table("users").Where("id", u.ID).
		Update(map[string]any{"updated_at": before.Add(-3600 * 1e9)})
	require.NoError(t, err)
	require.NoError(t, database.Load(u)) // no-op load keeps struct

	require.NoError(t, database.Touch(u))
	loaded, err := database.Find[User](u.ID)
	require.NoError(t, err)
	assert.False(t, loaded.UpdatedAt.Before(before), "updated_at bumped to now")
	assert.Equal(t, u.UpdatedAt.UTC(), loaded.UpdatedAt.UTC(), "struct field matches the row")
}
