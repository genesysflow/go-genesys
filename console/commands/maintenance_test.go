package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/genesysflow/go-genesys/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDownAndUpCommands(t *testing.T) {
	app := testutil.NewMockApplication()
	downFile := filepath.Join(t.TempDir(), "down")

	down := DownCommand(app)
	down.SetArgs([]string{"--file", downFile, "--message", "Upgrading", "--retry", "300", "--secret", "s3cret"})
	require.NoError(t, down.Execute())

	content, err := os.ReadFile(downFile)
	require.NoError(t, err)
	assert.Contains(t, string(content), "Upgrading")
	assert.Contains(t, string(content), "300")
	assert.Contains(t, string(content), "s3cret")

	up := UpCommand(app)
	up.SetArgs([]string{"--file", downFile})
	require.NoError(t, up.Execute())
	_, err = os.Stat(downFile)
	assert.True(t, os.IsNotExist(err))

	// up is idempotent.
	up = UpCommand(app)
	up.SetArgs([]string{"--file", downFile})
	require.NoError(t, up.Execute())
}
