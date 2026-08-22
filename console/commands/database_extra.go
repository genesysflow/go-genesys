package commands

import (
	"fmt"
	"sort"
	"strings"

	"github.com/genesysflow/go-genesys/console/cli"
	"github.com/genesysflow/go-genesys/container"
	"github.com/genesysflow/go-genesys/contracts"
	"github.com/genesysflow/go-genesys/database"
	"github.com/genesysflow/go-genesys/events"
	"github.com/genesysflow/go-genesys/schedule"
)

// MigrateRefreshCommand rolls every migration back and runs them again,
// Laravel's `artisan migrate:refresh`. Unlike migrate:fresh it goes
// through each migration's Down, so a Down that does not undo its Up is
// caught here rather than in production.
func MigrateRefreshCommand(app contracts.Application) *cli.Command {
	return &cli.Command{
		Name:        "migrate:refresh",
		Description: "Rollback and re-run all migrations",
		Options: []cli.Option{
			{Name: "seed", Description: "Seed the database afterwards"},
		},
		Handle: func(c *cli.Context) error {
			migrator, err := resolveMigrator(c.App())
			if err != nil {
				return err
			}

			rolledBack, err := migrator.Reset()
			if err != nil {
				return fmt.Errorf("migrate:refresh: rolling back: %w", err)
			}
			for _, name := range rolledBack {
				c.Line("Rolled back: " + name)
			}

			migrated, err := migrator.Run()
			if err != nil {
				return fmt.Errorf("migrate:refresh: migrating: %w", err)
			}
			for _, name := range migrated {
				c.Line("Migrated: " + name)
			}

			if c.BoolOption("seed") {
				return c.Call("db:seed")
			}

			c.Info("Database refreshed.")
			return nil
		},
	}
}

// DbWipeCommand drops every table in the database, Laravel's
// `artisan db:wipe`.
func DbWipeCommand(app contracts.Application) *cli.Command {
	return &cli.Command{
		Name:        "db:wipe",
		Description: "Drop all tables in the database",
		Options: []cli.Option{
			{Name: "force", Description: "Run without confirming, including in production"},
		},
		Handle: func(c *cli.Context) error {
			if !c.BoolOption("force") {
				// Dropping every table is unrecoverable, so production
				// needs an explicit --force rather than a prompt someone
				// can answer on autopilot.
				if c.App().IsProduction() {
					return fmt.Errorf("db:wipe: refusing to wipe a production database; pass --force if you mean it")
				}
				if !c.Confirm("Drop every table in the database?", false) {
					c.Line("Aborted.")
					return nil
				}
			}

			conn, driver, err := resolveConnection(c.App())
			if err != nil {
				return err
			}

			tables, err := listTables(conn, driver)
			if err != nil {
				return fmt.Errorf("db:wipe: %w", err)
			}
			if len(tables) == 0 {
				c.Line("No tables to drop.")
				return nil
			}

			for _, table := range tables {
				if _, err := conn.Exec(dropTableSQL(driver, table)); err != nil {
					return fmt.Errorf("db:wipe: dropping %s: %w", table, err)
				}
				c.Line("Dropped: " + table)
			}

			c.Info(fmt.Sprintf("Dropped %d tables.", len(tables)))
			return nil
		},
	}
}

// DbShowCommand summarises the connection and its tables, Laravel's
// `artisan db:show`.
func DbShowCommand(app contracts.Application) *cli.Command {
	return &cli.Command{
		Name:        "db:show",
		Description: "Display information about the database connection and its tables",
		Handle: func(c *cli.Context) error {
			manager, err := container.Resolve[*database.Manager](c.App())
			if err != nil {
				return fmt.Errorf("db:show: no database configured: %w", err)
			}

			name := manager.GetDefaultConnection()
			conn := manager.Connection()
			if connErr := conn.Error(); connErr != nil {
				return fmt.Errorf("db:show: %w", connErr)
			}
			driver := conn.Driver()

			c.Table([]string{"Property", "Value"}, [][]string{
				{"Connection", name},
				{"Driver", driver},
			})
			c.NewLine()

			tables, err := listTables(conn, driver)
			if err != nil {
				return fmt.Errorf("db:show: %w", err)
			}

			rows := make([][]string, 0, len(tables))
			for _, table := range tables {
				rows = append(rows, []string{table, fmt.Sprint(countRows(conn, table))})
			}
			c.Table([]string{"Table", "Rows"}, rows)
			return nil
		},
	}
}

// DbTableCommand describes one table's columns, Laravel's
// `artisan db:table`.
func DbTableCommand(app contracts.Application) *cli.Command {
	return &cli.Command{
		Name:        "db:table",
		Description: "Display the columns of a database table",
		Arguments: []cli.Argument{
			{Name: "table", Description: "The table to describe", Required: true},
		},
		Handle: func(c *cli.Context) error {
			conn, driver, err := resolveConnection(c.App())
			if err != nil {
				return err
			}

			table := c.Argument("table")
			columns, err := describeTable(conn, driver, table)
			if err != nil {
				return fmt.Errorf("db:table: %w", err)
			}
			if len(columns) == 0 {
				return fmt.Errorf("db:table: table %q does not exist", table)
			}

			c.Linef("Table: %s (%d rows)", table, countRows(conn, table))
			c.NewLine()
			c.Table([]string{"Column", "Type", "Nullable"}, columns)
			return nil
		},
	}
}

// resolveConnection returns the default connection and its driver.
func resolveConnection(app contracts.Application) (contracts.Connection, string, error) {
	manager, err := container.Resolve[*database.Manager](app)
	if err != nil {
		return nil, "", fmt.Errorf("no database configured - register the DatabaseServiceProvider: %w", err)
	}

	conn := manager.Connection()
	if connErr := conn.Error(); connErr != nil {
		return nil, "", fmt.Errorf("the database connection could not be opened: %w", connErr)
	}
	return conn, conn.Driver(), nil
}

// listTables returns the table names in the current schema, sorted.
func listTables(conn contracts.Connection, driver string) ([]string, error) {
	var query string
	switch driver {
	case "sqlite", "sqlite3":
		query = `SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name`
	case "postgres", "pgx":
		query = `SELECT tablename FROM pg_catalog.pg_tables WHERE schemaname = current_schema() ORDER BY tablename`
	case "mysql", "mariadb":
		query = `SELECT table_name FROM information_schema.tables WHERE table_schema = DATABASE() ORDER BY table_name`
	default:
		return nil, fmt.Errorf("listing tables is not supported for driver %q", driver)
	}

	rows, err := conn.Query(query)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		tables = append(tables, name)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	sort.Strings(tables)
	return tables, nil
}

// dropTableSQL renders a driver-appropriate DROP TABLE.
func dropTableSQL(driver, table string) string {
	quoted := quoteIdentifier(driver, table)
	if driver == "postgres" || driver == "pgx" {
		// CASCADE so foreign keys do not dictate the drop order.
		return "DROP TABLE IF EXISTS " + quoted + " CASCADE"
	}
	return "DROP TABLE IF EXISTS " + quoted
}

// quoteIdentifier quotes a table name for the driver. Names come from
// the database itself, but quoting keeps reserved words working.
func quoteIdentifier(driver, name string) string {
	switch driver {
	case "mysql", "mariadb":
		return "`" + strings.ReplaceAll(name, "`", "``") + "`"
	default:
		return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
	}
}

// describeTable returns column name/type/nullable rows for a table.
func describeTable(conn contracts.Connection, driver, table string) ([][]string, error) {
	switch driver {
	case "sqlite", "sqlite3":
		rows, err := conn.Query("PRAGMA table_info(" + quoteIdentifier(driver, table) + ")")
		if err != nil {
			return nil, err
		}
		defer func() { _ = rows.Close() }()

		var columns [][]string
		for rows.Next() {
			var (
				cid        int
				name, kind string
				notNull    int
				dflt       any
				pk         int
			)
			if err := rows.Scan(&cid, &name, &kind, &notNull, &dflt, &pk); err != nil {
				return nil, err
			}
			columns = append(columns, []string{name, kind, boolWord(notNull == 0)})
		}
		return columns, rows.Err()

	case "postgres", "pgx", "mysql", "mariadb":
		rows, err := conn.Query(
			`SELECT column_name, data_type, is_nullable
			 FROM information_schema.columns
			 WHERE table_name = ?
			 ORDER BY ordinal_position`,
			table,
		)
		if err != nil {
			return nil, err
		}
		defer func() { _ = rows.Close() }()

		var columns [][]string
		for rows.Next() {
			var name, kind, nullable string
			if err := rows.Scan(&name, &kind, &nullable); err != nil {
				return nil, err
			}
			columns = append(columns, []string{name, kind, boolWord(strings.EqualFold(nullable, "YES"))})
		}
		return columns, rows.Err()

	default:
		return nil, fmt.Errorf("describing tables is not supported for driver %q", driver)
	}
}

// countRows returns a table's row count, or -1 when it cannot be read.
func countRows(conn contracts.Connection, table string) int64 {
	var count int64
	row := conn.QueryRow("SELECT COUNT(*) FROM " + quoteIdentifier(conn.Driver(), table))
	if err := row.Scan(&count); err != nil {
		return -1
	}
	return count
}

func boolWord(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

// ScheduleTestCommand runs one scheduled task immediately, Laravel's
// `artisan schedule:test`. It ignores the cron expression: the point is
// to exercise the task without waiting for it to come due.
func ScheduleTestCommand(app contracts.Application) *cli.Command {
	return &cli.Command{
		Name:        "schedule:test",
		Description: "Run a scheduled task immediately, whether or not it is due",
		Arguments: []cli.Argument{
			{Name: "task", Description: "The task description to run", Required: true},
		},
		Handle: func(c *cli.Context) error {
			scheduler, err := container.Resolve[*schedule.Schedule](c.App())
			if err != nil {
				return fmt.Errorf("schedule:test: no scheduler available - register the ScheduleServiceProvider: %w", err)
			}

			wanted := c.Argument("task")
			for _, event := range scheduler.Events() {
				if event.GetDescription() != wanted {
					continue
				}

				c.Linef("Running [%s]...", wanted)
				if err := event.Run(); err != nil {
					return fmt.Errorf("schedule:test: task %q failed: %w", wanted, err)
				}
				c.Info(fmt.Sprintf("Task [%s] ran successfully.", wanted))
				return nil
			}

			return fmt.Errorf("schedule:test: no scheduled task described as %q", wanted)
		},
	}
}

// EventListCommand lists registered event listeners, Laravel's
// `artisan event:list`.
func EventListCommand(app contracts.Application) *cli.Command {
	return &cli.Command{
		Name:        "event:list",
		Description: "List the registered event listeners",
		Handle: func(c *cli.Context) error {
			dispatcher, err := container.Resolve[*events.Dispatcher](c.App())
			if err != nil {
				return fmt.Errorf("event:list: no dispatcher available - register the EventServiceProvider: %w", err)
			}

			counts := dispatcher.ListenerCounts()
			if len(counts) == 0 {
				c.Line("No event listeners are registered.")
				return nil
			}

			names := make([]string, 0, len(counts))
			for name := range counts {
				names = append(names, name)
			}
			sort.Strings(names)

			rows := make([][]string, 0, len(names))
			for _, name := range names {
				rows = append(rows, []string{name, fmt.Sprint(counts[name])})
			}

			c.Table([]string{"Event", "Listeners"}, rows)

			if wildcards := dispatcher.WildcardCount(); wildcards > 0 {
				c.NewLine()
				c.Linef("%d wildcard listener(s) receive every event.", wildcards)
			}
			return nil
		},
	}
}
