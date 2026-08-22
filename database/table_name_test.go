package database_test

import (
	"testing"

	"github.com/genesysflow/go-genesys/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type tableNamed struct {
	database.Model
	Name string `db:"name"`
}

// TableNameOf answers for a value, where TableNameFor needs a type -
// which is what a polymorphic column has to write when all it holds is
// an interface.
func TestTableNameOf(t *testing.T) {
	name, err := database.TableNameOf(&tableNamed{})
	require.NoError(t, err)
	assert.Equal(t, "table_nameds", name)

	// A value rather than a pointer answers the same.
	name, err = database.TableNameOf(tableNamed{})
	require.NoError(t, err)
	assert.Equal(t, "table_nameds", name)
}

// Anything that is not a model is an error, not a panic.
func TestTableNameOfRejectsNonModels(t *testing.T) {
	_, err := database.TableNameOf("not a model")
	assert.Error(t, err)

	_, err = database.TableNameOf(nil)
	assert.Error(t, err)
}
