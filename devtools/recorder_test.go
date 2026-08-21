package devtools_test

import (
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/genesysflow/go-genesys/database"
	"github.com/genesysflow/go-genesys/devtools"
	"github.com/genesysflow/go-genesys/foundation"
	genhttp "github.com/genesysflow/go-genesys/http"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func devKernel(t *testing.T) (*genhttp.Kernel, *devtools.Recorder) {
	t.Helper()

	app := foundation.New()
	require.NoError(t, app.Boot())

	kernel := genhttp.NewKernel(app, genhttp.KernelConfig{DisableStartupMessage: true})
	recorder := devtools.NewRecorder(10)
	kernel.Use(devtools.Middleware(recorder))

	return kernel, recorder
}

func TestRecordsRequests(t *testing.T) {
	kernel, recorder := devKernel(t)

	kernel.GET("/users", func(ctx *genhttp.Context) error {
		return ctx.JSONResponse(map[string]any{"ok": true})
	})
	kernel.GET("/missing", func(ctx *genhttp.Context) error {
		return ctx.Status(404).String("nope")
	})

	_, err := kernel.Fiber().Test(httptest.NewRequest("GET", "/users", nil), -1)
	require.NoError(t, err)
	_, err = kernel.Fiber().Test(httptest.NewRequest("GET", "/missing", nil), -1)
	require.NoError(t, err)

	requests := recorder.Requests()
	require.Len(t, requests, 2)

	assert.Equal(t, "GET", requests[0].Method)
	assert.Equal(t, "/users", requests[0].Path)
	assert.Equal(t, 200, requests[0].Status)
	assert.Greater(t, requests[0].Duration, time.Duration(0))
	assert.Equal(t, 404, requests[1].Status)
}

// The recorder is a ring buffer: a long-running dev server must not grow
// without bound.
func TestRecorderCapacity(t *testing.T) {
	recorder := devtools.NewRecorder(3)

	for i := 0; i < 10; i++ {
		recorder.RecordRequest(devtools.RequestEntry{Method: "GET", Path: "/p", Status: 200})
	}

	assert.Len(t, recorder.Requests(), 3)
}

func TestRecordsQueries(t *testing.T) {
	recorder := devtools.NewRecorder(10)

	recorder.RecordQuery(database.QueryEvent{
		Connection: "default",
		SQL:        "SELECT * FROM users WHERE email = ?",
		Bindings:   []any{"ada@example.com"},
		Duration:   2 * time.Millisecond,
	})

	queries := recorder.Queries()
	require.Len(t, queries, 1)
	assert.Contains(t, queries[0].SQL, "SELECT")
	assert.Equal(t, 1, queries[0].BindingCount)
}

// Bindings routinely carry personal data, so their values are withheld
// unless the developer asks for them.
func TestQueryBindingsAreRedactedByDefault(t *testing.T) {
	recorder := devtools.NewRecorder(10)
	recorder.RecordQuery(database.QueryEvent{
		SQL:      "SELECT * FROM users WHERE email = ?",
		Bindings: []any{"ada@example.com"},
	})

	assert.Empty(t, recorder.Queries()[0].Bindings)

	shown := devtools.NewRecorder(10)
	shown.ShowBindings = true
	shown.RecordQuery(database.QueryEvent{
		SQL:      "SELECT * FROM users WHERE email = ?",
		Bindings: []any{"ada@example.com"},
	})

	assert.Equal(t, []string{"ada@example.com"}, shown.Queries()[0].Bindings)
}

func TestQueriesDuringARequest(t *testing.T) {
	recorder := devtools.NewRecorder(10)

	start := time.Now()
	recorder.RecordQuery(database.QueryEvent{SQL: "SELECT 1"})
	entry := devtools.RequestEntry{Method: "GET", Path: "/", Status: 200, StartedAt: start, Duration: time.Since(start)}

	during := recorder.QueriesDuring(entry)
	assert.Len(t, during, 1)

	// A query from before the request is not attributed to it.
	old := devtools.RequestEntry{StartedAt: start.Add(time.Hour), Duration: time.Millisecond}
	assert.Empty(t, recorder.QueriesDuring(old))
}

func TestClear(t *testing.T) {
	recorder := devtools.NewRecorder(10)
	recorder.RecordRequest(devtools.RequestEntry{Path: "/"})
	recorder.RecordQuery(database.QueryEvent{SQL: "SELECT 1"})

	recorder.Clear()

	assert.Empty(t, recorder.Requests())
	assert.Empty(t, recorder.Queries())
}

// The recorder is written from every request handler at once.
func TestRecorderIsConcurrencySafe(t *testing.T) {
	recorder := devtools.NewRecorder(100)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			recorder.RecordRequest(devtools.RequestEntry{Path: "/p", Status: 200})
			recorder.RecordQuery(database.QueryEvent{SQL: "SELECT 1"})
			_ = recorder.Requests()
		}()
	}
	wg.Wait()

	assert.Len(t, recorder.Requests(), 50)
	assert.Len(t, recorder.Queries(), 50)
}

// --- panel -----------------------------------------------------------

func TestPanelRendersRecordedEntries(t *testing.T) {
	kernel, recorder := devKernel(t)
	require.NoError(t, devtools.Register(kernel, recorder, "/_genesys"))

	kernel.GET("/users", func(ctx *genhttp.Context) error { return ctx.String("ok") })
	_, err := kernel.Fiber().Test(httptest.NewRequest("GET", "/users", nil), -1)
	require.NoError(t, err)

	recorder.RecordQuery(database.QueryEvent{SQL: "SELECT * FROM users", Duration: time.Millisecond})

	resp, err := kernel.Fiber().Test(httptest.NewRequest("GET", "/_genesys", nil), -1)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)

	body := make([]byte, 8192)
	n, _ := resp.Body.Read(body)
	rendered := string(body[:n])

	assert.Contains(t, rendered, "/users")
	assert.Contains(t, rendered, "SELECT * FROM users")
}

// SQL and request paths are exactly what must not be served from a
// production box, so the panel refuses to register there.
func TestPanelRefusesInProduction(t *testing.T) {
	t.Setenv("APP_ENV", "production")

	app := foundation.New()
	require.NoError(t, app.Boot())
	require.True(t, app.IsProduction())

	kernel := genhttp.NewKernel(app, genhttp.KernelConfig{DisableStartupMessage: true})
	recorder := devtools.NewRecorder(10)

	err := devtools.Register(kernel, recorder, "/_genesys")
	require.Error(t, err)

	resp, err := kernel.Fiber().Test(httptest.NewRequest("GET", "/_genesys", nil), -1)
	require.NoError(t, err)
	assert.Equal(t, 404, resp.StatusCode, "the panel must not be reachable in production")
}

// The panel does not record itself, which would otherwise fill the
// buffer with its own visits.
func TestPanelDoesNotRecordItself(t *testing.T) {
	kernel, recorder := devKernel(t)
	require.NoError(t, devtools.Register(kernel, recorder, "/_genesys"))

	_, err := kernel.Fiber().Test(httptest.NewRequest("GET", "/_genesys", nil), -1)
	require.NoError(t, err)

	for _, entry := range recorder.Requests() {
		assert.False(t, strings.HasPrefix(entry.Path, "/_genesys"), "the panel should not record itself")
	}
}
