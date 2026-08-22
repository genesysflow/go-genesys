package database

import (
	"testing"

	"github.com/genesysflow/go-genesys/query"
	"github.com/stretchr/testify/assert"
)

func TestBuildDSNMySQL(t *testing.T) {
	dsn := buildDSN(ConnectionConfig{
		Driver:   "mysql",
		Host:     "db.internal",
		Port:     3307,
		Database: "app",
		Username: "app_user",
		Password: "s3cret",
	})
	assert.Equal(t,
		"app_user:s3cret@tcp(db.internal:3307)/app?parseTime=true&charset=utf8mb4&loc=UTC&clientFoundRows=true", dsn)

	// The default port fills in, and mariadb builds the same DSN shape.
	dsn = buildDSN(ConnectionConfig{
		Driver: "mariadb", Host: "localhost", Database: "app", Username: "u", Password: "p",
	})
	assert.Contains(t, dsn, "@tcp(localhost:3306)/app")
}

func TestMapDriverMySQL(t *testing.T) {
	assert.Equal(t, "mysql", mapDriver("mysql"))
	assert.Equal(t, "mysql", mapDriver("mariadb"))
	assert.Equal(t, "postgres", mapDriver("pgsql"))
	assert.Equal(t, "sqlite", mapDriver("sqlite"))
}

func TestMySQLGrammarQuoting(t *testing.T) {
	for _, driver := range []string{"mysql", "mariadb"} {
		sqlStr, bindings := query.New(driver, nil).Table("users").
			Where("active", true).
			WhereIn("role", "admin", "editor").
			OrderBy("name").
			Limit(10).
			ToSQL()
		assert.Equal(t,
			"SELECT * FROM `users` WHERE `active` = ? AND `role` IN (?, ?) ORDER BY `name` ASC LIMIT 10",
			sqlStr, "driver %s", driver)
		assert.Equal(t, []any{true, "admin", "editor"}, bindings)
	}
}

// Two MySQL DSN parameters decide whether the query layer's protection
// survives, and both are dangerous by their presence rather than their
// absence - so this pins them.
//
//   - interpolateParams=true moves parameter substitution into the
//     client, so a query stops using real prepared statements and starts
//     relying on the driver's escaping being right for the connection's
//     charset.
//   - multiStatements=true lets one call run several statements, which
//     turns any mistake in the SQL into arbitrary execution.
//
// Neither is set, and neither should be added for convenience.
func TestMySQLDSNDoesNotWeakenParameterBinding(t *testing.T) {
	dsn := buildDSN(ConnectionConfig{
		Driver:   "mysql",
		Host:     "127.0.0.1",
		Database: "app",
		Username: "root",
	})

	assert.NotContains(t, dsn, "interpolateParams",
		"parameters must be bound by the server, not interpolated by the client")
	assert.NotContains(t, dsn, "multiStatements",
		"one call must not be able to run several statements")
}
