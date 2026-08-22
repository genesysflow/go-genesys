package validation_test

import (
	"database/sql"
	"testing"

	"github.com/genesysflow/go-genesys/query"
	"github.com/genesysflow/go-genesys/validation"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

// usersDB returns a validator wired to a sqlite database holding two users.
func usersDB(t *testing.T) *validation.Validator {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec(`CREATE TABLE users (id INTEGER PRIMARY KEY, email TEXT, team_id INTEGER)`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO users (id, email, team_id) VALUES (1, 'ada@example.com', 7), (2, 'grace@example.com', 7)`)
	require.NoError(t, err)

	v := validation.New()
	v.SetDatabaseResolver(func() (string, query.Executor, error) {
		return "sqlite", db, nil
	})
	return v
}

// --- unique ----------------------------------------------------------

type signupRequest struct {
	Email string `json:"email" validate:"required,unique=users.email"`
}

func TestUniqueRejectsExistingValue(t *testing.T) {
	result := usersDB(t).Validate(&signupRequest{Email: "ada@example.com"})

	require.True(t, result.Fails())
	assert.Contains(t, result.FirstFor("email"), "already been taken")
}

func TestUniqueAllowsNewValue(t *testing.T) {
	result := usersDB(t).Validate(&signupRequest{Email: "alan@example.com"})
	assert.True(t, result.Passes(), result.All())
}

// Updating a record must not trip over the record's own row, which is
// what Laravel's ignore-id argument is for.
func TestUniqueIgnoresGivenID(t *testing.T) {
	v := usersDB(t)

	result := v.ValidateMap(
		map[string]any{"email": "ada@example.com"},
		map[string]string{"email": "unique=users.email.1"},
	)
	assert.True(t, result.Passes(), result.All())

	// Ignoring a different row still catches the clash.
	result = v.ValidateMap(
		map[string]any{"email": "ada@example.com"},
		map[string]string{"email": "unique=users.email.2"},
	)
	assert.True(t, result.Fails())
}

// The ignored column defaults to id but can be named explicitly.
func TestUniqueIgnoresGivenColumn(t *testing.T) {
	v := usersDB(t)

	result := v.ValidateMap(
		map[string]any{"email": "ada@example.com"},
		map[string]string{"email": "unique=users.email.7.team_id"},
	)
	assert.True(t, result.Passes(), result.All())
}

// --- exists ----------------------------------------------------------

func TestExists(t *testing.T) {
	v := usersDB(t)

	result := v.ValidateMap(
		map[string]any{"email": "ada@example.com"},
		map[string]string{"email": "exists=users.email"},
	)
	assert.True(t, result.Passes(), result.All())

	result = v.ValidateMap(
		map[string]any{"email": "nobody@example.com"},
		map[string]string{"email": "exists=users.email"},
	)
	require.True(t, result.Fails())
	assert.Contains(t, result.FirstFor("email"), "is invalid")
}

// --- fail closed -----------------------------------------------------

// With no database wired up, a uniqueness check cannot be answered.
// Passing would wave through the duplicate the rule exists to stop, so
// the rule fails closed.
func TestDatabaseRulesFailWithoutDatabase(t *testing.T) {
	v := validation.New()

	result := v.ValidateMap(
		map[string]any{"email": "ada@example.com"},
		map[string]string{"email": "unique=users.email"},
	)
	assert.True(t, result.Fails(), "unique must not pass when it cannot check")

	result = v.ValidateMap(
		map[string]any{"email": "ada@example.com"},
		map[string]string{"email": "exists=users.email"},
	)
	assert.True(t, result.Fails(), "exists must not pass when it cannot check")
}

// Table and column names come from the rule, so anything that is not a
// plain identifier is refused rather than interpolated into SQL.
func TestDatabaseRulesRejectNonIdentifiers(t *testing.T) {
	v := usersDB(t)

	result := v.ValidateMap(
		map[string]any{"email": "ada@example.com"},
		map[string]string{"email": `exists=users.email" OR "1`},
	)
	assert.True(t, result.Fails())
}

// --- confirmed -------------------------------------------------------

type passwordRequest struct {
	Password             string `json:"password" validate:"required,confirmed"`
	PasswordConfirmation string `json:"password_confirmation"`
}

func TestConfirmed(t *testing.T) {
	v := validation.New()

	result := v.Validate(&passwordRequest{Password: "hunter2", PasswordConfirmation: "hunter2"})
	assert.True(t, result.Passes(), result.All())

	result = v.Validate(&passwordRequest{Password: "hunter2", PasswordConfirmation: "typo"})
	require.True(t, result.Fails())
	assert.Contains(t, result.FirstFor("password"), "confirmation does not match")

	result = v.Validate(&passwordRequest{Password: "hunter2"})
	assert.True(t, result.Fails(), "a missing confirmation must fail")
}

// --- prohibited ------------------------------------------------------

type prohibitedRequest struct {
	Role string `json:"role" validate:"prohibited"`
}

func TestProhibited(t *testing.T) {
	v := validation.New()

	assert.True(t, v.Validate(&prohibitedRequest{}).Passes())

	result := v.Validate(&prohibitedRequest{Role: "admin"})
	require.True(t, result.Fails())
	assert.Contains(t, result.FirstFor("role"), "prohibited")
}

// --- built-in unique is preserved ------------------------------------

type tagsRequest struct {
	Tags []string `json:"tags" validate:"unique"`
}

type membersRequest struct {
	Members []member `json:"members" validate:"unique=Email"`
}

type member struct {
	Email string
}

// go-playground's unique checks distinct slice elements. The database
// rule reuses the tag name, so the original meaning must survive for
// params that are not table.column.
func TestUniqueKeepsSliceSemantics(t *testing.T) {
	v := validation.New()

	assert.True(t, v.Validate(&tagsRequest{Tags: []string{"a", "b"}}).Passes())
	assert.True(t, v.Validate(&tagsRequest{Tags: []string{"a", "a"}}).Fails())

	assert.True(t, v.Validate(&membersRequest{Members: []member{{"a@x.com"}, {"b@x.com"}}}).Passes())
	assert.True(t, v.Validate(&membersRequest{Members: []member{{"a@x.com"}, {"a@x.com"}}}).Fails())
}

// --- conditional required messages -----------------------------------

type conditionalRequest struct {
	Kind string `json:"kind" validate:"required"`
	VAT  string `json:"vat" validate:"required_if=Kind company"`
}

func TestRequiredIfMessage(t *testing.T) {
	v := validation.New()

	assert.True(t, v.Validate(&conditionalRequest{Kind: "individual"}).Passes())

	result := v.Validate(&conditionalRequest{Kind: "company"})
	require.True(t, result.Fails())
	assert.Contains(t, result.FirstFor("vat"), "required")
	assert.NotContains(t, result.FirstFor("vat"), "failed validation")
}
