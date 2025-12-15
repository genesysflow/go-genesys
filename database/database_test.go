package database_test

import (
	"os"
	"testing"

	"github.com/genesysflow/go-genesys/contracts"
	"github.com/genesysflow/go-genesys/database"
	"github.com/genesysflow/go-genesys/facades/db"

	_ "github.com/mattn/go-sqlite3"
)

func setupTestDB(t *testing.T) (*database.Manager, func()) {
	t.Helper()

	// Create temp database file
	tmpFile, err := os.CreateTemp("", "test_*.db")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tmpFile.Close()

	config := database.Config{
		Default: "sqlite",
		Connections: map[string]database.ConnectionConfig{
			"sqlite": {
				Driver:                "sqlite",
				Database:              tmpFile.Name(),
				ForeignKeyConstraints: true,
			},
		},
	}

	manager := database.NewManager(config)

	// Initialize DB facade
	db.SetInstance(manager)

	// Create test table
	conn := manager.Connection()
	if conn == nil {
		os.Remove(tmpFile.Name())
		t.Fatalf("Failed to get connection")
	}

	_, err = conn.Exec(`
		CREATE TABLE users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			email TEXT UNIQUE NOT NULL,
			age INTEGER,
			status TEXT DEFAULT 'active',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		os.Remove(tmpFile.Name())
		t.Fatalf("Failed to create test table: %v", err)
	}

	// Create posts table for join tests
	_, err = conn.Exec(`
		CREATE TABLE posts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			title TEXT NOT NULL,
			content TEXT,
			FOREIGN KEY (user_id) REFERENCES users(id)
		)
	`)
	if err != nil {
		os.Remove(tmpFile.Name())
		t.Fatalf("Failed to create posts table: %v", err)
	}

	cleanup := func() {
		manager.Close()
		os.Remove(tmpFile.Name())
	}

	return manager, cleanup
}

func TestConnection(t *testing.T) {
	manager, cleanup := setupTestDB(t)
	defer cleanup()

	t.Run("GetConnection", func(t *testing.T) {
		conn := manager.Connection()
		if conn == nil {
			t.Fatal("Expected connection, got nil")
		}
		if conn.Driver() != "sqlite" {
			t.Errorf("Expected driver 'sqlite', got '%s'", conn.Driver())
		}
	})

	t.Run("Ping", func(t *testing.T) {
		conn := manager.Connection()
		if err := conn.Ping(); err != nil {
			t.Errorf("Ping failed: %v", err)
		}
	})

	t.Run("DefaultConnection", func(t *testing.T) {
		if manager.GetDefaultConnection() != "sqlite" {
			t.Errorf("Expected default connection 'sqlite', got '%s'", manager.GetDefaultConnection())
		}
	})
}

func TestInsert(t *testing.T) {
	_, cleanup := setupTestDB(t)
	defer cleanup()

	t.Run("InsertSingle", func(t *testing.T) {
		affected, err := db.Table("users").Insert(map[string]any{
			"name":  "John Doe",
			"email": "john@example.com",
			"age":   30,
		})
		if err != nil {
			t.Fatalf("Insert failed: %v", err)
		}
		if affected != 1 {
			t.Errorf("Expected 1 row affected, got %d", affected)
		}
	})

	t.Run("InsertGetId", func(t *testing.T) {
		id, err := db.Table("users").InsertGetId(map[string]any{
			"name":  "Jane Doe",
			"email": "jane@example.com",
			"age":   25,
		})
		if err != nil {
			t.Fatalf("InsertGetId failed: %v", err)
		}
		if id <= 0 {
			t.Errorf("Expected positive ID, got %d", id)
		}
	})

	t.Run("InsertBatch", func(t *testing.T) {
		affected, err := db.Table("users").InsertBatch([]map[string]any{
			{"name": "User 1", "email": "user1@example.com", "age": 20},
			{"name": "User 2", "email": "user2@example.com", "age": 21},
			{"name": "User 3", "email": "user3@example.com", "age": 22},
		})
		if err != nil {
			t.Fatalf("InsertBatch failed: %v", err)
		}
		if affected != 3 {
			t.Errorf("Expected 3 rows affected, got %d", affected)
		}
	})
}

func TestSelect(t *testing.T) {
	_, cleanup := setupTestDB(t)
	defer cleanup()

	// Insert test data
	db.Table("users").Insert(map[string]any{"name": "Alice", "email": "alice@test.com", "age": 25, "status": "active"})
	db.Table("users").Insert(map[string]any{"name": "Bob", "email": "bob@test.com", "age": 30, "status": "inactive"})
	db.Table("users").Insert(map[string]any{"name": "Charlie", "email": "charlie@test.com", "age": 35, "status": "active"})

	t.Run("GetAll", func(t *testing.T) {
		results, err := db.Table("users").Get()
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		if len(results) != 3 {
			t.Errorf("Expected 3 results, got %d", len(results))
		}
	})

	t.Run("SelectColumns", func(t *testing.T) {
		results, err := db.Table("users").Select("name", "email").Get()
		if err != nil {
			t.Fatalf("Select failed: %v", err)
		}
		if len(results) != 3 {
			t.Errorf("Expected 3 results, got %d", len(results))
		}
		// Check that only selected columns are present
		if _, ok := results[0]["name"]; !ok {
			t.Error("Expected 'name' column in results")
		}
	})

	t.Run("First", func(t *testing.T) {
		result, err := db.Table("users").First()
		if err != nil {
			t.Fatalf("First failed: %v", err)
		}
		if result == nil {
			t.Fatal("Expected result, got nil")
		}
	})

	t.Run("Find", func(t *testing.T) {
		result, err := db.Table("users").Find(1)
		if err != nil {
			t.Fatalf("Find failed: %v", err)
		}
		if result == nil {
			t.Fatal("Expected result, got nil")
		}
		if result["name"] != "Alice" {
			t.Errorf("Expected name 'Alice', got '%v'", result["name"])
		}
	})

	t.Run("Value", func(t *testing.T) {
		value, err := db.Table("users").Where("id", "=", 1).Value("name")
		if err != nil {
			t.Fatalf("Value failed: %v", err)
		}
		if value != "Alice" {
			t.Errorf("Expected 'Alice', got '%v'", value)
		}
	})

	t.Run("Pluck", func(t *testing.T) {
		names, err := db.Table("users").Pluck("name")
		if err != nil {
			t.Fatalf("Pluck failed: %v", err)
		}
		if len(names) != 3 {
			t.Errorf("Expected 3 names, got %d", len(names))
		}
	})
}

func TestWhere(t *testing.T) {
	_, cleanup := setupTestDB(t)
	defer cleanup()

	// Insert test data
	db.Table("users").Insert(map[string]any{"name": "Alice", "email": "alice@test.com", "age": 25, "status": "active"})
	db.Table("users").Insert(map[string]any{"name": "Bob", "email": "bob@test.com", "age": 30, "status": "inactive"})
	db.Table("users").Insert(map[string]any{"name": "Charlie", "email": "charlie@test.com", "age": 35, "status": "active"})

	t.Run("WhereBasic", func(t *testing.T) {
		results, err := db.Table("users").Where("status", "=", "active").Get()
		if err != nil {
			t.Fatalf("Where failed: %v", err)
		}
		if len(results) != 2 {
			t.Errorf("Expected 2 results, got %d", len(results))
		}
	})

	t.Run("WhereMultiple", func(t *testing.T) {
		results, err := db.Table("users").
			Where("status", "=", "active").
			Where("age", ">", 30).
			Get()
		if err != nil {
			t.Fatalf("Where failed: %v", err)
		}
		if len(results) != 1 {
			t.Errorf("Expected 1 result, got %d", len(results))
		}
	})

	t.Run("OrWhere", func(t *testing.T) {
		results, err := db.Table("users").
			Where("name", "=", "Alice").
			OrWhere("name", "=", "Bob").
			Get()
		if err != nil {
			t.Fatalf("OrWhere failed: %v", err)
		}
		if len(results) != 2 {
			t.Errorf("Expected 2 results, got %d", len(results))
		}
	})

	t.Run("WhereIn", func(t *testing.T) {
		results, err := db.Table("users").
			WhereIn("name", []any{"Alice", "Charlie"}).
			Get()
		if err != nil {
			t.Fatalf("WhereIn failed: %v", err)
		}
		if len(results) != 2 {
			t.Errorf("Expected 2 results, got %d", len(results))
		}
	})

	t.Run("WhereNotIn", func(t *testing.T) {
		results, err := db.Table("users").
			WhereNotIn("name", []any{"Alice", "Bob"}).
			Get()
		if err != nil {
			t.Fatalf("WhereNotIn failed: %v", err)
		}
		if len(results) != 1 {
			t.Errorf("Expected 1 result, got %d", len(results))
		}
	})

	t.Run("WhereBetween", func(t *testing.T) {
		results, err := db.Table("users").
			WhereBetween("age", 25, 32).
			Get()
		if err != nil {
			t.Fatalf("WhereBetween failed: %v", err)
		}
		if len(results) != 2 {
			t.Errorf("Expected 2 results, got %d", len(results))
		}
	})

	t.Run("WhereNull", func(t *testing.T) {
		// Insert user with null age
		db.Table("users").Insert(map[string]any{"name": "NullAge", "email": "null@test.com"})

		results, err := db.Table("users").WhereNull("age").Get()
		if err != nil {
			t.Fatalf("WhereNull failed: %v", err)
		}
		if len(results) != 1 {
			t.Errorf("Expected 1 result, got %d", len(results))
		}
	})

	t.Run("WhereNotNull", func(t *testing.T) {
		results, err := db.Table("users").WhereNotNull("age").Get()
		if err != nil {
			t.Fatalf("WhereNotNull failed: %v", err)
		}
		if len(results) != 3 {
			t.Errorf("Expected 3 results, got %d", len(results))
		}
	})
}

func TestOrderBy(t *testing.T) {
	_, cleanup := setupTestDB(t)
	defer cleanup()

	db.Table("users").Insert(map[string]any{"name": "Charlie", "email": "c@test.com", "age": 35})
	db.Table("users").Insert(map[string]any{"name": "Alice", "email": "a@test.com", "age": 25})
	db.Table("users").Insert(map[string]any{"name": "Bob", "email": "b@test.com", "age": 30})

	t.Run("OrderByAsc", func(t *testing.T) {
		results, err := db.Table("users").OrderBy("name", "asc").Get()
		if err != nil {
			t.Fatalf("OrderBy failed: %v", err)
		}
		if results[0]["name"] != "Alice" {
			t.Errorf("Expected first result to be Alice, got %v", results[0]["name"])
		}
	})

	t.Run("OrderByDesc", func(t *testing.T) {
		results, err := db.Table("users").OrderByDesc("age").Get()
		if err != nil {
			t.Fatalf("OrderByDesc failed: %v", err)
		}
		if results[0]["name"] != "Charlie" {
			t.Errorf("Expected first result to be Charlie, got %v", results[0]["name"])
		}
	})
}

func TestLimitOffset(t *testing.T) {
	_, cleanup := setupTestDB(t)
	defer cleanup()

	for i := 1; i <= 10; i++ {
		db.Table("users").Insert(map[string]any{
			"name":  "User" + string(rune('0'+i)),
			"email": "user" + string(rune('0'+i)) + "@test.com",
			"age":   20 + i,
		})
	}

	t.Run("Limit", func(t *testing.T) {
		results, err := db.Table("users").Limit(5).Get()
		if err != nil {
			t.Fatalf("Limit failed: %v", err)
		}
		if len(results) != 5 {
			t.Errorf("Expected 5 results, got %d", len(results))
		}
	})

	t.Run("LimitOffset", func(t *testing.T) {
		results, err := db.Table("users").Limit(3).Offset(2).Get()
		if err != nil {
			t.Fatalf("Limit/Offset failed: %v", err)
		}
		if len(results) != 3 {
			t.Errorf("Expected 3 results, got %d", len(results))
		}
	})

	t.Run("ForPage", func(t *testing.T) {
		results, err := db.Table("users").ForPage(2, 3).Get()
		if err != nil {
			t.Fatalf("ForPage failed: %v", err)
		}
		if len(results) != 3 {
			t.Errorf("Expected 3 results, got %d", len(results))
		}
	})

	t.Run("Take", func(t *testing.T) {
		results, err := db.Table("users").Take(2).Get()
		if err != nil {
			t.Fatalf("Take failed: %v", err)
		}
		if len(results) != 2 {
			t.Errorf("Expected 2 results, got %d", len(results))
		}
	})
}

func TestAggregates(t *testing.T) {
	_, cleanup := setupTestDB(t)
	defer cleanup()

	db.Table("users").Insert(map[string]any{"name": "A", "email": "a@test.com", "age": 20})
	db.Table("users").Insert(map[string]any{"name": "B", "email": "b@test.com", "age": 30})
	db.Table("users").Insert(map[string]any{"name": "C", "email": "c@test.com", "age": 40})

	t.Run("Count", func(t *testing.T) {
		count, err := db.Table("users").Count()
		if err != nil {
			t.Fatalf("Count failed: %v", err)
		}
		if count != 3 {
			t.Errorf("Expected count 3, got %d", count)
		}
	})

	t.Run("CountWithWhere", func(t *testing.T) {
		count, err := db.Table("users").Where("age", ">", 25).Count()
		if err != nil {
			t.Fatalf("Count failed: %v", err)
		}
		if count != 2 {
			t.Errorf("Expected count 2, got %d", count)
		}
	})

	t.Run("Sum", func(t *testing.T) {
		sum, err := db.Table("users").Sum("age")
		if err != nil {
			t.Fatalf("Sum failed: %v", err)
		}
		if sum != 90 {
			t.Errorf("Expected sum 90, got %v", sum)
		}
	})

	t.Run("Avg", func(t *testing.T) {
		avg, err := db.Table("users").Avg("age")
		if err != nil {
			t.Fatalf("Avg failed: %v", err)
		}
		if avg != 30 {
			t.Errorf("Expected avg 30, got %v", avg)
		}
	})

	t.Run("Exists", func(t *testing.T) {
		exists, err := db.Table("users").Where("name", "=", "A").Exists()
		if err != nil {
			t.Fatalf("Exists failed: %v", err)
		}
		if !exists {
			t.Error("Expected exists to be true")
		}
	})

	t.Run("DoesntExist", func(t *testing.T) {
		doesntExist, err := db.Table("users").Where("name", "=", "X").DoesntExist()
		if err != nil {
			t.Fatalf("DoesntExist failed: %v", err)
		}
		if !doesntExist {
			t.Error("Expected doesntExist to be true")
		}
	})
}

func TestUpdate(t *testing.T) {
	_, cleanup := setupTestDB(t)
	defer cleanup()

	db.Table("users").Insert(map[string]any{"name": "Alice", "email": "alice@test.com", "age": 25})

	t.Run("UpdateBasic", func(t *testing.T) {
		affected, err := db.Table("users").Where("name", "=", "Alice").Update(map[string]any{
			"age": 26,
		})
		if err != nil {
			t.Fatalf("Update failed: %v", err)
		}
		if affected != 1 {
			t.Errorf("Expected 1 row affected, got %d", affected)
		}

		// Verify update
		user, _ := db.Table("users").Where("name", "=", "Alice").First()
		if user["age"] != int64(26) {
			t.Errorf("Expected age 26, got %v", user["age"])
		}
	})

	t.Run("Increment", func(t *testing.T) {
		_, err := db.Table("users").Where("name", "=", "Alice").Increment("age", 5)
		if err != nil {
			t.Fatalf("Increment failed: %v", err)
		}

		user, _ := db.Table("users").Where("name", "=", "Alice").First()
		if user["age"] != int64(31) {
			t.Errorf("Expected age 31, got %v", user["age"])
		}
	})

	t.Run("Decrement", func(t *testing.T) {
		_, err := db.Table("users").Where("name", "=", "Alice").Decrement("age", 1)
		if err != nil {
			t.Fatalf("Decrement failed: %v", err)
		}

		user, _ := db.Table("users").Where("name", "=", "Alice").First()
		if user["age"] != int64(30) {
			t.Errorf("Expected age 30, got %v", user["age"])
		}
	})
}

func TestDelete(t *testing.T) {
	_, cleanup := setupTestDB(t)
	defer cleanup()

	db.Table("users").Insert(map[string]any{"name": "Alice", "email": "alice@test.com"})
	db.Table("users").Insert(map[string]any{"name": "Bob", "email": "bob@test.com"})

	t.Run("DeleteWithWhere", func(t *testing.T) {
		affected, err := db.Table("users").Where("name", "=", "Alice").Delete()
		if err != nil {
			t.Fatalf("Delete failed: %v", err)
		}
		if affected != 1 {
			t.Errorf("Expected 1 row affected, got %d", affected)
		}

		count, _ := db.Table("users").Count()
		if count != 1 {
			t.Errorf("Expected 1 user remaining, got %d", count)
		}
	})
}

func TestJoins(t *testing.T) {
	_, cleanup := setupTestDB(t)
	defer cleanup()

	// Insert users
	db.Table("users").Insert(map[string]any{"name": "Alice", "email": "alice@test.com", "age": 25})
	db.Table("users").Insert(map[string]any{"name": "Bob", "email": "bob@test.com", "age": 30})

	// Insert posts
	db.Table("posts").Insert(map[string]any{"user_id": 1, "title": "Alice's Post 1", "content": "Content 1"})
	db.Table("posts").Insert(map[string]any{"user_id": 1, "title": "Alice's Post 2", "content": "Content 2"})
	db.Table("posts").Insert(map[string]any{"user_id": 2, "title": "Bob's Post", "content": "Content 3"})

	t.Run("InnerJoin", func(t *testing.T) {
		results, err := db.Table("users").
			Select("users.name", "posts.title").
			Join("posts", "users.id", "=", "posts.user_id").
			Get()
		if err != nil {
			t.Fatalf("Join failed: %v", err)
		}
		if len(results) != 3 {
			t.Errorf("Expected 3 results, got %d", len(results))
		}
	})

	t.Run("LeftJoin", func(t *testing.T) {
		// Add user without posts
		db.Table("users").Insert(map[string]any{"name": "Charlie", "email": "charlie@test.com", "age": 35})

		results, err := db.Table("users").
			Select("users.name", "posts.title").
			LeftJoin("posts", "users.id", "=", "posts.user_id").
			Get()
		if err != nil {
			t.Fatalf("LeftJoin failed: %v", err)
		}
		if len(results) != 4 { // 3 posts + 1 user without posts
			t.Errorf("Expected 4 results, got %d", len(results))
		}
	})
}

func TestTransactions(t *testing.T) {
	manager, cleanup := setupTestDB(t)
	defer cleanup()

	t.Run("TransactionCommit", func(t *testing.T) {
		err := manager.Transaction(func(tx contracts.Transaction) error {
			tx.Table("users").Insert(map[string]any{"name": "TxUser", "email": "tx@test.com"})
			return nil // commit
		})
		if err != nil {
			t.Fatalf("Transaction failed: %v", err)
		}

		count, _ := db.Table("users").Where("name", "=", "TxUser").Count()
		if count != 1 {
			t.Errorf("Expected 1 user after commit, got %d", count)
		}
	})

	t.Run("TransactionRollback", func(t *testing.T) {
		err := manager.Transaction(func(tx contracts.Transaction) error {
			tx.Table("users").Insert(map[string]any{"name": "RollbackUser", "email": "rollback@test.com"})
			return os.ErrInvalid // trigger rollback
		})
		if err == nil {
			t.Fatal("Expected error, got nil")
		}

		count, _ := db.Table("users").Where("name", "=", "RollbackUser").Count()
		if count != 0 {
			t.Errorf("Expected 0 users after rollback, got %d", count)
		}
	})
}

func TestDistinct(t *testing.T) {
	_, cleanup := setupTestDB(t)
	defer cleanup()

	db.Table("users").Insert(map[string]any{"name": "Alice", "email": "a1@test.com", "status": "active"})
	db.Table("users").Insert(map[string]any{"name": "Bob", "email": "b1@test.com", "status": "active"})
	db.Table("users").Insert(map[string]any{"name": "Charlie", "email": "c1@test.com", "status": "inactive"})

	t.Run("Distinct", func(t *testing.T) {
		results, err := db.Table("users").Select("status").Distinct().Get()
		if err != nil {
			t.Fatalf("Distinct failed: %v", err)
		}
		if len(results) != 2 {
			t.Errorf("Expected 2 distinct statuses, got %d", len(results))
		}
	})
}

func TestToSQL(t *testing.T) {
	_, cleanup := setupTestDB(t)
	defer cleanup()

	t.Run("GenerateSQL", func(t *testing.T) {
		sql, bindings := db.Table("users").
			Select("name", "email").
			Where("status", "=", "active").
			OrderBy("name", "asc").
			Limit(10).
			ToSQL()

		if sql == "" {
			t.Error("Expected SQL string, got empty")
		}
		if len(bindings) != 1 {
			t.Errorf("Expected 1 binding, got %d", len(bindings))
		}
		if bindings[0] != "active" {
			t.Errorf("Expected binding 'active', got '%v'", bindings[0])
		}
	})
}

func TestClone(t *testing.T) {
	_, cleanup := setupTestDB(t)
	defer cleanup()

	db.Table("users").Insert(map[string]any{"name": "Alice", "email": "a@test.com", "age": 25})
	db.Table("users").Insert(map[string]any{"name": "Bob", "email": "b@test.com", "age": 30})

	t.Run("CloneBuilder", func(t *testing.T) {
		baseQuery := db.Table("users").Where("age", ">", 20)

		// Clone and add more conditions
		clone1 := baseQuery.Clone().Where("name", "=", "Alice")
		clone2 := baseQuery.Clone().Where("name", "=", "Bob")

		results1, _ := clone1.Get()
		results2, _ := clone2.Get()

		if len(results1) != 1 {
			t.Errorf("Clone1: Expected 1 result, got %d", len(results1))
		}
		if len(results2) != 1 {
			t.Errorf("Clone2: Expected 1 result, got %d", len(results2))
		}
	})
}
