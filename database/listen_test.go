package database_test

import (
	"sync"
	"testing"
	"time"

	"github.com/genesysflow/go-genesys/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func TestQueryListener(t *testing.T) {
	setupORM(t)
	manager := database.Default()

	var mu sync.Mutex
	var events []database.QueryEvent
	manager.Listen(func(e database.QueryEvent) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, e)
	})
	t.Cleanup(manager.ClearListeners)

	require.NoError(t, database.Create(&User{Name: "Q", Email: "q@x.io"}))
	_, err := database.Query[User]().Where("name", "Q").Get()
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	require.NotEmpty(t, events)

	var sawInsert, sawSelect bool
	for _, event := range events {
		assert.Equal(t, "test", event.Connection)
		assert.GreaterOrEqual(t, event.Duration, time.Duration(0))
		switch {
		case containsSQL(event.SQL, "INSERT INTO"):
			sawInsert = true
			assert.Contains(t, event.Bindings, "Q")
		case containsSQL(event.SQL, "SELECT"):
			sawSelect = true
		}
	}
	assert.True(t, sawInsert, "insert was observed")
	assert.True(t, sawSelect, "select was observed")
}

func TestQueryListenerSeesErrors(t *testing.T) {
	setupORM(t)
	manager := database.Default()

	var lastErr error
	manager.Listen(func(e database.QueryEvent) {
		if e.Err != nil {
			lastErr = e.Err
		}
	})
	t.Cleanup(manager.ClearListeners)

	_, err := manager.Statement("SELECT * FROM missing_table")
	require.Error(t, err)
	assert.Equal(t, err, lastErr, "listener saw the failing query's error")
}

func containsSQL(sql, fragment string) bool {
	return len(sql) >= len(fragment) && (sql[:len(fragment)] == fragment ||
		len(sql) > len(fragment) && stringContains(sql, fragment))
}

func stringContains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
