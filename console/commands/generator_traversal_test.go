package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/genesysflow/go-genesys/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Generator names carrying path separators must not escape the target
// directory.
func TestGeneratorNamesCannotTraversePaths(t *testing.T) {
	app := testutil.NewMockApplication()
	base := t.TempDir()
	app.SetBasePath(base)

	cmd := MakeJobCommand(app)
	cmd.SetArgs([]string{"../../outside/evil"})
	require.NoError(t, cmd.Execute())

	// Nothing may be written outside base/app/jobs.
	_, err := os.Stat(filepath.Join(base, "..", "outside"))
	assert.True(t, os.IsNotExist(err), "no file may escape the project tree")

	entries, err := os.ReadDir(filepath.Join(base, "app", "jobs"))
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "outsideevil_job.go", entries[0].Name())
}
