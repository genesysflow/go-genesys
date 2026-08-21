package db_test

import (
	"testing"

	"github.com/genesysflow/go-genesys/database"
	"github.com/genesysflow/go-genesys/facades/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func TestTableFacade(t *testing.T) {
	manager := database.NewManager(database.Config{
		Default: "test",
		Connections: map[string]database.ConnectionConfig{
			"test": {Driver: "sqlite", Database: ":memory:"},
		},
	})
	t.Cleanup(func() {
		db.SetInstance(nil)
		manager.Close()
	})
	db.SetInstance(manager)

	_, err := db.Statement(`CREATE TABLE things (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT)`)
	require.NoError(t, err)

	require.NoError(t, db.Table("things").Insert(map[string]any{"name": "widget"}))
	rows, err := db.Table("things").Where("name", "widget").Get()
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "widget", rows[0]["name"])

	count, err := db.Table("things").Count()
	require.NoError(t, err)
	assert.EqualValues(t, 1, count)
}

func TestTableFacadePanicsWithoutInstance(t *testing.T) {
	db.SetInstance(nil)
	assert.Panics(t, func() { db.Table("things") })
}
