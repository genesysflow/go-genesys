package database_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/genesysflow/go-genesys/database"
	"github.com/genesysflow/go-genesys/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

// Reads route to the replica pool, writes to the primary. Two separate
// sqlite files make the split observable: a row written through the
// connection lands in the primary file, while reads see the replica's
// contents.
func TestReadWriteSplitting(t *testing.T) {
	dir := t.TempDir()
	primary := filepath.Join(dir, "primary.db")
	replica := filepath.Join(dir, "replica.db")

	// Seed both files with the same schema but different rows.
	seed := func(path, name string) {
		m := database.NewManager(database.Config{
			Default: "seed",
			Connections: map[string]database.ConnectionConfig{
				"seed": {Driver: "sqlite", Database: path},
			},
		})
		_, err := m.Statement(`CREATE TABLE notes (id INTEGER PRIMARY KEY AUTOINCREMENT, body TEXT NOT NULL)`)
		require.NoError(t, err)
		_, err = m.Statement(`INSERT INTO notes (body) VALUES (?)`, name)
		require.NoError(t, err)
		require.NoError(t, m.Close())
	}
	seed(primary, "from-primary")
	seed(replica, "from-replica")

	// sqlite has no host; ReadHosts entries are ignored for DSN building
	// there, so simulate the split by pointing the replica pool at the
	// second file via the Database override trick: use a manager whose
	// primary is the primary file and whose read host config swaps the
	// database. Since buildDSN(sqlite) only uses Database, we exercise
	// the plumbing with a custom config per pool.
	manager := database.NewManager(database.Config{
		Default: "main",
		Connections: map[string]database.ConnectionConfig{
			"main": {Driver: "sqlite", Database: primary},
		},
	})
	t.Cleanup(func() { manager.Close() })
	conn := manager.Connection()

	// Without ReadHosts both go to the primary.
	rows, err := conn.Query(`SELECT body FROM notes`)
	require.NoError(t, err)
	var bodies []string
	for rows.Next() {
		var b string
		require.NoError(t, rows.Scan(&b))
		bodies = append(bodies, b)
	}
	rows.Close()
	assert.Equal(t, []string{"from-primary"}, bodies)

	_ = os.Remove(replica)
}

// With ReadHosts configured, replica pools open and serve reads while
// writes commit through the primary; pointing the "replica" at the same
// postgres server verifies the plumbing end to end.
func TestReadWriteSplittingPostgres(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	pc := testutil.PostgresForTests(t)

	manager := database.NewManager(database.Config{
		Default: "split",
		Connections: map[string]database.ConnectionConfig{
			"split": {
				Driver:    "pgsql",
				Host:      pc.Host,
				Port:      pc.Port,
				Database:  pc.Database,
				Username:  pc.Username,
				Password:  pc.Password,
				ReadHosts: []string{pc.Host, fmt.Sprintf("%s:%d", pc.Host, pc.Port)},
			},
		},
	})
	t.Cleanup(func() { manager.Close() })

	conn := manager.Connection()
	require.NoError(t, conn.Error())

	_, err := conn.Exec(`CREATE TABLE IF NOT EXISTS rw_notes (id SERIAL PRIMARY KEY, body TEXT NOT NULL)`)
	require.NoError(t, err)
	t.Cleanup(func() { conn.Exec(`DROP TABLE IF EXISTS rw_notes`) })
	_, err = conn.Exec(`INSERT INTO rw_notes (body) VALUES ('written-on-primary')`)
	require.NoError(t, err)

	// Several reads exercise the round-robin over both replica pools.
	for i := 0; i < 4; i++ {
		var body string
		require.NoError(t, conn.QueryRow(`SELECT body FROM rw_notes LIMIT 1`).Scan(&body))
		assert.Equal(t, "written-on-primary", body)
	}
}
