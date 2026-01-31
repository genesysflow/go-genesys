package schema

import (
	"testing"

	"github.com/genesysflow/go-genesys/database"
	"github.com/genesysflow/go-genesys/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "github.com/lib/pq"
)

// newTestDatabaseManager creates a database.Manager configured to use the test container.
func newTestDatabaseManager(pc *testutil.PostgresContainer) *database.Manager {
	cfg := database.Config{
		Default: "default",
		Connections: map[string]database.ConnectionConfig{
			"default": {
				Driver:   "pgsql",
				Host:     pc.Host,
				Port:     pc.Port,
				Database: pc.Database,
				Username: pc.Username,
				Password: pc.Password,
				SSLMode:  "disable",
			},
		},
	}
	return database.NewManager(cfg)
}

func TestNewBuilder(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	pc, cleanup := testutil.SetupPostgresContainer(t)
	defer cleanup()

	manager := newTestDatabaseManager(pc)
	defer manager.Close()

	conn := manager.Connection()
	db := conn.DB()

	builder := NewBuilder(db, "postgres")
	assert.NotNil(t, builder)
}

func TestBuilderCreate(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	pc, cleanup := testutil.SetupPostgresContainer(t)
	defer cleanup()

	manager := newTestDatabaseManager(pc)
	defer manager.Close()

	conn := manager.Connection()
	db := conn.DB()

	builder := NewBuilder(db, "postgres")

	err := builder.Create("test_create", func(bp *Blueprint) {
		bp.ID()
		bp.String("name")
		bp.String("email", 100)
	})
	require.NoError(t, err)

	assert.True(t, builder.HasTable("test_create"))
}

func TestBuilderDrop(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	pc, cleanup := testutil.SetupPostgresContainer(t)
	defer cleanup()

	manager := newTestDatabaseManager(pc)
	defer manager.Close()

	conn := manager.Connection()
	db := conn.DB()

	builder := NewBuilder(db, "postgres")

	// Create table first
	err := builder.Create("test_drop", func(bp *Blueprint) {
		bp.ID()
	})
	require.NoError(t, err)
	assert.True(t, builder.HasTable("test_drop"))

	// Drop it
	err = builder.Drop("test_drop")
	require.NoError(t, err)
	assert.False(t, builder.HasTable("test_drop"))
}

func TestBuilderDropIfExists(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	pc, cleanup := testutil.SetupPostgresContainer(t)
	defer cleanup()

	manager := newTestDatabaseManager(pc)
	defer manager.Close()

	conn := manager.Connection()
	db := conn.DB()

	builder := NewBuilder(db, "postgres")

	// Should not error even if table doesn't exist
	err := builder.DropIfExists("nonexistent_table")
	require.NoError(t, err)

	// Create and drop
	err = builder.Create("test_drop_if_exists", func(bp *Blueprint) {
		bp.ID()
	})
	require.NoError(t, err)

	err = builder.DropIfExists("test_drop_if_exists")
	require.NoError(t, err)
	assert.False(t, builder.HasTable("test_drop_if_exists"))
}

func TestBuilderRename(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	pc, cleanup := testutil.SetupPostgresContainer(t)
	defer cleanup()

	manager := newTestDatabaseManager(pc)
	defer manager.Close()

	conn := manager.Connection()
	db := conn.DB()

	builder := NewBuilder(db, "postgres")

	// Create table
	err := builder.Create("original_name", func(bp *Blueprint) {
		bp.ID()
	})
	require.NoError(t, err)

	// Rename
	err = builder.Rename("original_name", "new_name")
	require.NoError(t, err)

	assert.False(t, builder.HasTable("original_name"))
	assert.True(t, builder.HasTable("new_name"))
}

func TestBuilderHasTable(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	pc, cleanup := testutil.SetupPostgresContainer(t)
	defer cleanup()

	manager := newTestDatabaseManager(pc)
	defer manager.Close()

	conn := manager.Connection()
	db := conn.DB()

	builder := NewBuilder(db, "postgres")

	assert.False(t, builder.HasTable("nonexistent"))

	err := builder.Create("exists_test", func(bp *Blueprint) {
		bp.ID()
	})
	require.NoError(t, err)

	assert.True(t, builder.HasTable("exists_test"))
}

func TestBlueprintID(t *testing.T) {
	bp := NewBlueprint("test")
	col := bp.ID()
	
	assert.Equal(t, "id", col.Name)
	assert.Equal(t, "integer", col.Type)
	assert.True(t, col.AutoIncrement)
	assert.True(t, col.Primary)
}

func TestBlueprintIDWithCustomName(t *testing.T) {
	bp := NewBlueprint("test")
	col := bp.ID("custom_id")
	
	assert.Equal(t, "custom_id", col.Name)
}

func TestBlueprintBigIncrements(t *testing.T) {
	bp := NewBlueprint("test")
	col := bp.BigIncrements("id")
	
	assert.Equal(t, "bigint", col.Type)
	assert.True(t, col.AutoIncrement)
	assert.True(t, col.Primary)
}

func TestBlueprintString(t *testing.T) {
	bp := NewBlueprint("test")
	
	// Default length
	col1 := bp.String("name")
	assert.Equal(t, "varchar", col1.Type)
	assert.Equal(t, 255, col1.Length)
	
	// Custom length
	col2 := bp.String("code", 50)
	assert.Equal(t, 50, col2.Length)
}

func TestBlueprintText(t *testing.T) {
	bp := NewBlueprint("test")
	col := bp.Text("description")
	
	assert.Equal(t, "text", col.Type)
}

func TestBlueprintInteger(t *testing.T) {
	bp := NewBlueprint("test")
	col := bp.Integer("count")
	
	assert.Equal(t, "integer", col.Type)
}

func TestBlueprintBigInteger(t *testing.T) {
	bp := NewBlueprint("test")
	col := bp.BigInteger("big_count")
	
	assert.Equal(t, "bigint", col.Type)
}

func TestBlueprintBoolean(t *testing.T) {
	bp := NewBlueprint("test")
	col := bp.Boolean("is_active")
	
	assert.Equal(t, "boolean", col.Type)
}

func TestBlueprintDecimal(t *testing.T) {
	bp := NewBlueprint("test")
	col := bp.Decimal("price", 10, 2)
	
	assert.Equal(t, "decimal", col.Type)
	assert.Equal(t, 10, col.Precision)
	assert.Equal(t, 2, col.Scale)
}

func TestBlueprintFloat(t *testing.T) {
	bp := NewBlueprint("test")
	col := bp.Float("rating")
	
	assert.Equal(t, "float", col.Type)
}

func TestBlueprintDateTime(t *testing.T) {
	bp := NewBlueprint("test")
	col := bp.DateTime("event_at")
	
	assert.Equal(t, "datetime", col.Type)
}

func TestBlueprintTimestamp(t *testing.T) {
	bp := NewBlueprint("test")
	col := bp.Timestamp("created_at")
	
	assert.Equal(t, "timestamp", col.Type)
}

func TestBlueprintTimestamps(t *testing.T) {
	bp := NewBlueprint("test")
	bp.Timestamps()
	
	assert.Len(t, bp.columns, 2)
	assert.Equal(t, "created_at", bp.columns[0].Name)
	assert.Equal(t, "updated_at", bp.columns[1].Name)
	assert.True(t, bp.columns[0].IsNullable)
	assert.True(t, bp.columns[1].IsNullable)
}

func TestBlueprintSoftDeletes(t *testing.T) {
	bp := NewBlueprint("test")
	bp.SoftDeletes()
	
	assert.Len(t, bp.columns, 1)
	assert.Equal(t, "deleted_at", bp.columns[0].Name)
	assert.True(t, bp.columns[0].IsNullable)
}

func TestBlueprintForeignID(t *testing.T) {
	bp := NewBlueprint("test")
	col := bp.ForeignID("user_id")
	
	assert.Equal(t, "bigint", col.Type)
	assert.True(t, col.Unsigned)
}

func TestBlueprintIndex(t *testing.T) {
	bp := NewBlueprint("test")
	bp.Index("name", "email")
	
	assert.Len(t, bp.indexes, 1)
	assert.Equal(t, []string{"name", "email"}, bp.indexes[0].Columns)
	assert.Equal(t, "INDEX", bp.indexes[0].Type)
}

func TestBlueprintUnique(t *testing.T) {
	bp := NewBlueprint("test")
	bp.Unique("email")
	
	assert.Len(t, bp.indexes, 1)
	assert.Equal(t, "UNIQUE", bp.indexes[0].Type)
}

func TestBlueprintPrimary(t *testing.T) {
	bp := NewBlueprint("test")
	bp.Primary("id", "tenant_id")
	
	assert.Len(t, bp.indexes, 1)
	assert.Equal(t, "PRIMARY", bp.indexes[0].Type)
	assert.Equal(t, []string{"id", "tenant_id"}, bp.indexes[0].Columns)
}

func TestColumnNullable(t *testing.T) {
	bp := NewBlueprint("test")
	col := bp.String("name").Nullable()
	
	assert.True(t, col.IsNullable)
}

func TestColumnDefault(t *testing.T) {
	bp := NewBlueprint("test")
	col := bp.String("status").Default("pending")
	
	assert.Equal(t, "pending", col.DefaultValue)
}

func TestColumnUnique(t *testing.T) {
	bp := NewBlueprint("test")
	col := bp.String("email").Unique()
	
	assert.True(t, col.IsUnique)
}

func TestColumnIndex(t *testing.T) {
	bp := NewBlueprint("test")
	col := bp.String("slug").Index()
	
	assert.True(t, col.IsIndex)
}

func TestColumnComment(t *testing.T) {
	bp := NewBlueprint("test")
	col := bp.String("status").Comment("User status")
	
	assert.Equal(t, "User status", col.ColumnComment)
}

func TestColumnChaining(t *testing.T) {
	bp := NewBlueprint("test")
	col := bp.String("email").Nullable().Unique().Comment("User email")
	
	assert.True(t, col.IsNullable)
	assert.True(t, col.IsUnique)
	assert.Equal(t, "User email", col.ColumnComment)
}

func TestCompleteTable(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	pc, cleanup := testutil.SetupPostgresContainer(t)
	defer cleanup()

	manager := newTestDatabaseManager(pc)
	defer manager.Close()

	conn := manager.Connection()
	db := conn.DB()

	builder := NewBuilder(db, "postgres")

	err := builder.Create("users", func(bp *Blueprint) {
		bp.ID()
		bp.String("name")
		bp.String("email", 100).Unique()
		bp.String("password")
		bp.Text("bio").Nullable()
		bp.Boolean("is_active").Default(true)
		bp.Integer("role_id").Nullable()
		bp.Timestamps()
		bp.SoftDeletes()
	})
	require.NoError(t, err)
	assert.True(t, builder.HasTable("users"))

	// Test we can insert data
	_, err = db.Exec(`
		INSERT INTO users (name, email, password, is_active)
		VALUES ($1, $2, $3, $4)
	`, "John", "john@example.com", "hashed", true)
	require.NoError(t, err)

	// Test we can query
	var name string
	err = db.QueryRow("SELECT name FROM users WHERE email = $1", "john@example.com").Scan(&name)
	require.NoError(t, err)
	assert.Equal(t, "John", name)
}

func TestNewGrammar(t *testing.T) {
	pgGrammar := NewGrammar("postgres")
	assert.IsType(t, &PostgresGrammar{}, pgGrammar)

	pgsqlGrammar := NewGrammar("pgsql")
	assert.IsType(t, &PostgresGrammar{}, pgsqlGrammar)

	postgresqlGrammar := NewGrammar("postgresql")
	assert.IsType(t, &PostgresGrammar{}, postgresqlGrammar)

	sqliteGrammar := NewGrammar("sqlite")
	assert.IsType(t, &SQLiteGrammar{}, sqliteGrammar)

	unknownGrammar := NewGrammar("unknown")
	assert.IsType(t, &SQLiteGrammar{}, unknownGrammar) // defaults to SQLite
}

func TestPostgresGrammarWrapTable(t *testing.T) {
	g := &PostgresGrammar{}
	assert.Equal(t, `"users"`, g.WrapTable("users"))
}

func TestPostgresGrammarWrapColumn(t *testing.T) {
	g := &PostgresGrammar{}
	assert.Equal(t, `"name"`, g.WrapColumn("name"))
}

func TestSQLiteGrammarWrapTable(t *testing.T) {
	g := &SQLiteGrammar{}
	assert.Equal(t, `"users"`, g.WrapTable("users"))
}

func TestSQLiteGrammarWrapColumn(t *testing.T) {
	g := &SQLiteGrammar{}
	assert.Equal(t, `"name"`, g.WrapColumn("name"))
}
