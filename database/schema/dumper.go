package schema

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
)

// Dumper handles dumping the database schema to a file.
type Dumper struct {
	db     *sql.DB
	driver string
}

// NewDumper creates a new Dumper.
func NewDumper(db *sql.DB, driver string) *Dumper {
	return &Dumper{
		db:     db,
		driver: driver,
	}
}

// Dump dumps the schema to the specified file path.
func (d *Dumper) Dump(path string) error {
	var schema string
	var err error

	switch d.driver {
	case "sqlite", "sqlite3":
		schema, err = d.dumpSQLite()
	case "postgres", "pgsql", "postgresql":
		schema, err = d.dumpPostgres()
	default:
		return fmt.Errorf("driver %s not supported for schema dumping", d.driver)
	}

	if err != nil {
		return err
	}

	return os.WriteFile(path, []byte(schema), 0644)
}

// dumpSQLite dumps the schema for SQLite.
func (d *Dumper) dumpSQLite() (string, error) {
	// Query sqlite_master for schema
	// We exclude sqlite_sequence (internal) and migrations table
	query := `SELECT sql FROM sqlite_master WHERE sql IS NOT NULL AND name != 'sqlite_sequence' AND name != 'migrations' ORDER BY name`
	rows, err := d.db.Query(query)
	if err != nil {
		return "", fmt.Errorf("failed to query sqlite_master: %w", err)
	}
	defer rows.Close()

	var statements []string
	statements = append(statements, "-- Auto-generated schema dump", "")

	for rows.Next() {
		var sql string
		if err := rows.Scan(&sql); err != nil {
			return "", err
		}
		// Ensure semicolon at the end
		if !strings.HasSuffix(sql, ";") {
			sql += ";"
		}
		statements = append(statements, sql, "")
	}

	if err := rows.Err(); err != nil {
		return "", err
	}

	return strings.Join(statements, "\n"), nil
}

// dumpPostgres dumps the schema for PostgreSQL.
func (d *Dumper) dumpPostgres() (string, error) {
	// Get all tables
	query := `
		SELECT table_name
		FROM information_schema.tables
		WHERE table_schema = 'public'
		AND table_type = 'BASE TABLE'
		AND table_name != 'migrations'
		ORDER BY table_name
	`
	rows, err := d.db.Query(query)
	if err != nil {
		return "", fmt.Errorf("failed to query information_schema.tables: %w", err)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return "", err
		}
		tables = append(tables, name)
	}

	var statements []string
	statements = append(statements, "-- Auto-generated schema dump", "")

	for _, table := range tables {
		// Get primary keys
		pkQuery := `
			SELECT kcu.column_name
			FROM information_schema.table_constraints tc
			JOIN information_schema.key_column_usage kcu
			ON tc.constraint_name = kcu.constraint_name
			AND tc.table_schema = kcu.table_schema
			WHERE tc.constraint_type = 'PRIMARY KEY'
			AND tc.table_name = $1
			AND tc.table_schema = 'public'
		`
		pkRows, err := d.db.Query(pkQuery, table)
		if err != nil {
			return "", fmt.Errorf("failed to query primary keys for %s: %w", table, err)
		}
		defer pkRows.Close()

		pks := make(map[string]bool)
		for pkRows.Next() {
			var col string
			if err := pkRows.Scan(&col); err != nil {
				return "", err
			}
			pks[col] = true
		}

		// Get columns
		colQuery := `
			SELECT column_name, data_type, is_nullable, column_default, character_maximum_length
			FROM information_schema.columns
			WHERE table_name = $1 AND table_schema = 'public'
			ORDER BY ordinal_position
		`
		colRows, err := d.db.Query(colQuery, table)
		if err != nil {
			return "", fmt.Errorf("failed to query columns for %s: %w", table, err)
		}
		defer colRows.Close()

		var cols []string
		for colRows.Next() {
			var name, dataType, isNullable string
			var defaultVal *string
			var maxLength *int

			if err := colRows.Scan(&name, &dataType, &isNullable, &defaultVal, &maxLength); err != nil {
				return "", err
			}

			// Format column definition
			colDef := fmt.Sprintf(`"%s" %s`, name, dataType)

			if maxLength != nil && (dataType == "character varying" || dataType == "character") {
				colDef = fmt.Sprintf(`"%s" %s(%d)`, name, dataType, *maxLength)
			}

			if isNullable == "NO" {
				colDef += " NOT NULL"
			}

			// Handle DEFAULT
			if defaultVal != nil {
				// Simply append the default value
				// Note: this might include '::regclass' etc.
				colDef += fmt.Sprintf(" DEFAULT %s", *defaultVal)
			}

			// Inline PRIMARY KEY if single PK
			if len(pks) == 1 && pks[name] {
				colDef += " PRIMARY KEY"
				delete(pks, name) // Mark as handled
			}

			cols = append(cols, "  "+colDef)
		}

		stmt := fmt.Sprintf(`CREATE TABLE "%s" (`, table)
		stmt += "\n" + strings.Join(cols, ",\n")

		// Add composite PKs if any remaining
		if len(pks) > 0 {
			var pkCols []string
			for pk := range pks {
				pkCols = append(pkCols, fmt.Sprintf(`"%s"`, pk))
			}
			stmt += fmt.Sprintf(",\n  PRIMARY KEY (%s)", strings.Join(pkCols, ", "))
		}

		stmt += "\n);"
		statements = append(statements, stmt, "")
	}

	return strings.Join(statements, "\n"), nil
}
