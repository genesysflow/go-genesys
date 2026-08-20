package database_test

import (
	"strings"
	"testing"

	"github.com/genesysflow/go-genesys/database"
	"github.com/genesysflow/go-genesys/database/schema"
	"github.com/genesysflow/go-genesys/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

// --- schema: table-level constraints and indexes must reach the SQL ---

func TestCreateEmitsConstraintsAndIndexes(t *testing.T) {
	setupORM(t)
	db := database.Default().Connection().DB()
	builder := schema.NewBuilder(db, "sqlite")

	require.NoError(t, builder.Create("accounts", func(bp *schema.Blueprint) {
		bp.ID()
		bp.String("email", 255)
		bp.String("tenant", 64).Index()
		bp.Unique("email", "tenant")
	}))

	// The composite unique constraint actually constrains.
	_, err := db.Exec(`INSERT INTO accounts (email, tenant) VALUES ('a@x.io', 't1')`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO accounts (email, tenant) VALUES ('a@x.io', 't2')`)
	require.NoError(t, err, "different tenant is allowed")
	_, err = db.Exec(`INSERT INTO accounts (email, tenant) VALUES ('a@x.io', 't1')`)
	require.Error(t, err, "duplicate (email, tenant) must violate the unique constraint")

	// The column-level .Index() produced a real index.
	var count int
	require.NoError(t, db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='accounts_tenant_index'`).Scan(&count))
	assert.Equal(t, 1, count, "column-level .Index() must create an index")
}

func TestMySQLSchemaGrammar(t *testing.T) {
	g := schema.NewGrammar("mysql")
	bp := schema.NewBlueprint("users")
	bp.ID()
	bp.String("email", 191).Unique()
	bp.Boolean("active").Default(true)
	bp.Index("email")

	sql := g.CompileCreate(bp)
	assert.Contains(t, sql, "CREATE TABLE `users`")
	assert.Contains(t, sql, "`id` INT AUTO_INCREMENT PRIMARY KEY")
	assert.Contains(t, sql, "`email` VARCHAR(191) NOT NULL UNIQUE")
	assert.Contains(t, sql, "`active` TINYINT(1) NOT NULL DEFAULT 1")
	assert.NotContains(t, sql, `"users"`, "MySQL must not use double-quoted identifiers")
	assert.NotContains(t, sql, "AUTOINCREMENT", "AUTOINCREMENT is SQLite spelling")

	indexes := g.CompileCreateIndexes(bp)
	require.Len(t, indexes, 1)
	assert.Equal(t, "CREATE INDEX `users_email_index` ON `users` (`email`)", indexes[0])
}

// --- Has() on a self-referential relation must correlate ---

type TreeComment struct {
	database.Model
	Body     string         `db:"body"`
	ParentID int64          `db:"parent_id"`
	Replies  []*TreeComment `rel:"hasMany,fk:parent_id"`
}

func (TreeComment) TableName() string { return "tree_comments" }

func TestHasSelfReferentialRelation(t *testing.T) {
	setupORM(t)
	manager := database.Default()
	_, err := manager.Statement(`CREATE TABLE tree_comments (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		body TEXT NOT NULL,
		parent_id INTEGER NOT NULL DEFAULT 0,
		created_at TIMESTAMP,
		updated_at TIMESTAMP
	)`)
	require.NoError(t, err)

	root := &TreeComment{Body: "root"}
	require.NoError(t, database.Create(root))
	leaf := &TreeComment{Body: "leaf", ParentID: root.ID}
	require.NoError(t, database.Create(leaf))
	lonely := &TreeComment{Body: "lonely"}
	require.NoError(t, database.Create(lonely))

	withReplies, err := database.Query[TreeComment]().Has("Replies").Get()
	require.NoError(t, err)
	require.Len(t, withReplies, 1, "only the root has a reply")
	assert.Equal(t, "root", withReplies[0].Body)

	childless, err := database.Query[TreeComment]().DoesntHave("Replies").OrderBy("id").Get()
	require.NoError(t, err)
	require.Len(t, childless, 2)
	assert.Equal(t, "leaf", childless[0].Body)
	assert.Equal(t, "lonely", childless[1].Body)
}

// --- aggregates over DISTINCT ---

func TestCountRespectsDistinct(t *testing.T) {
	setupORM(t)
	for _, u := range []*User{
		{Name: "A", Email: "a@x.io", Age: 30},
		{Name: "B", Email: "b@x.io", Age: 30},
		{Name: "C", Email: "c@x.io", Age: 40},
	} {
		require.NoError(t, database.Create(u))
	}

	manager := database.Default()
	q := query.New("sqlite", manager.Connection()).Table("users").Distinct().Select("age")
	count, err := q.Count()
	require.NoError(t, err)
	assert.EqualValues(t, 2, count, "COUNT over a DISTINCT query counts distinct rows")
}

// --- Skip without Take must not emit bare OFFSET ---

func TestSkipWithoutLimit(t *testing.T) {
	setupORM(t)
	for _, u := range []*User{
		{Name: "A", Email: "a@x.io"},
		{Name: "B", Email: "b@x.io"},
		{Name: "C", Email: "c@x.io"},
	} {
		require.NoError(t, database.Create(u))
	}

	rows, err := query.New("sqlite", database.Default().Connection()).
		Table("users").OrderBy("id").Skip(1).Get()
	require.NoError(t, err, "OFFSET without LIMIT must still be valid SQL")
	assert.Len(t, rows, 2)
}

// --- outer fields override embedded columns ---

type StringIDRow struct {
	database.Model
	ID   string `db:"id"`
	Name string `db:"name"`
}

func (StringIDRow) TableName() string { return "string_id_rows" }

func TestOuterFieldOverridesEmbeddedColumn(t *testing.T) {
	setupORM(t)
	manager := database.Default()
	_, err := manager.Statement(`CREATE TABLE string_id_rows (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		created_at TIMESTAMP,
		updated_at TIMESTAMP
	)`)
	require.NoError(t, err)
	_, err = manager.Statement(`INSERT INTO string_id_rows (id, name) VALUES ('uuid-1', 'first')`)
	require.NoError(t, err)

	rows, err := database.Query[StringIDRow]().Get()
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "uuid-1", rows[0].ID, "the outer string ID field must receive the column")
	assert.Equal(t, "first", rows[0].Name)
}

// --- Chunk must not be derailed by a user ORDER BY ---

func TestChunkIgnoresUserOrdering(t *testing.T) {
	setupORM(t)
	names := []string{"Zoe", "Amy", "Mia", "Bea", "Lou"}
	for _, n := range names {
		require.NoError(t, database.Create(&User{Name: n, Email: strings.ToLower(n) + "@x.io"}))
	}

	var visited []string
	err := database.Query[User]().OrderByDesc("name").Chunk(2, func(users []User) error {
		for _, u := range users {
			visited = append(visited, u.Name)
		}
		return nil
	})
	require.NoError(t, err)
	assert.ElementsMatch(t, names, visited, "keyset chunking must visit every row exactly once")
}
