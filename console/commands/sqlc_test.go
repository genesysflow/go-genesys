package commands

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/genesysflow/go-genesys/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSqlcGenerateGuidesInstallationWhenMissing(t *testing.T) {
	original := sqlcLookPath
	t.Cleanup(func() { sqlcLookPath = original })
	sqlcLookPath = func(file string) (string, error) {
		return "", exec.ErrNotFound
	}

	cmd := SqlcGenerateCommand(testutil.NewMockApplication())
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not installed")
	assert.Contains(t, err.Error(), "go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest")
}

func TestSqlcGenerateRunsTheBinary(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fake binary")
	}

	// A fake sqlc that records its arguments.
	dir := t.TempDir()
	marker := filepath.Join(dir, "invoked")
	script := "#!/bin/sh\necho \"$@\" > " + marker + "\n"
	fake := filepath.Join(dir, "sqlc")
	require.NoError(t, os.WriteFile(fake, []byte(script), 0o755))

	original := sqlcLookPath
	t.Cleanup(func() { sqlcLookPath = original })
	sqlcLookPath = func(file string) (string, error) { return fake, nil }

	cmd := SqlcGenerateCommand(testutil.NewMockApplication())
	cmd.SetArgs([]string{"--no-remote"})
	require.NoError(t, cmd.Execute())

	recorded, err := os.ReadFile(marker)
	require.NoError(t, err)
	assert.Equal(t, "generate --no-remote\n", string(recorded))
}

func TestSqlcGenerateSurfacesFailures(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fake binary")
	}

	dir := t.TempDir()
	fake := filepath.Join(dir, "sqlc")
	require.NoError(t, os.WriteFile(fake, []byte("#!/bin/sh\nexit 3\n"), 0o755))

	original := sqlcLookPath
	t.Cleanup(func() { sqlcLookPath = original })
	sqlcLookPath = func(file string) (string, error) { return fake, nil }

	cmd := SqlcGenerateCommand(testutil.NewMockApplication())
	err := cmd.Execute()
	assert.ErrorContains(t, err, "sqlc generate failed")
}
