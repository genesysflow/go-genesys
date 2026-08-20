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
		"app_user:s3cret@tcp(db.internal:3307)/app?parseTime=true&charset=utf8mb4&loc=UTC", dsn)

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
