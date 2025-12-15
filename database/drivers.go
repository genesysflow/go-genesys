// Package database provides driver imports for database functionality.
// Import this package to automatically register database drivers.
package database

// Import drivers for side effects (registration).
// Uncomment the drivers you want to use:
import (
	_ "github.com/lib/pq"           // PostgreSQL driver
	_ "github.com/mattn/go-sqlite3" // SQLite driver
)
