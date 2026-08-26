package http_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net"
	nethttp "net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/genesysflow/go-genesys/contracts"
	"github.com/genesysflow/go-genesys/foundation"
	genhttp "github.com/genesysflow/go-genesys/http"
	"github.com/genesysflow/go-genesys/log"
	"github.com/genesysflow/go-genesys/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// syncBuffer collects log output written from the goroutines fasthttp
// serves connections on, which the race detector would otherwise catch
// reading from the test goroutine.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

// records decodes what has been logged so far, one map per record.
func (b *syncBuffer) records(t *testing.T) []map[string]any {
	t.Helper()

	b.mu.Lock()
	raw := b.buf.String()
	b.mu.Unlock()

	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		if line == "" {
			continue
		}
		var record map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &record), "log line is not JSON: %s", line)
		out = append(out, record)
	}
	return out
}

// recordsAt returns the logged records of one level.
func (b *syncBuffer) recordsAt(t *testing.T, level string) []map[string]any {
	t.Helper()

	var out []map[string]any
	for _, record := range b.records(t) {
		if record["level"] == level {
			out = append(out, record)
		}
	}
	return out
}

// serveForConnectionTests starts a kernel on a loopback listener with a
// read timeout short enough that a socket which never speaks is timed
// out well inside a test run, and returns its address alongside the log
// the kernel writes to.
func serveForConnectionTests(t *testing.T, readTimeout time.Duration) (string, *syncBuffer) {
	t.Helper()

	logs := &syncBuffer{}
	logger := log.NewJSON(logs)
	logger.SetLevel(contracts.LogLevelDebug)

	app := foundation.New()
	app.SetLogger(logger)
	require.NoError(t, app.Register(&providers.AppServiceProvider{}))
	require.NoError(t, app.Boot())

	kernel := genhttp.NewKernel(app, genhttp.KernelConfig{
		DisableStartupMessage: true,
		ReadTimeout:           readTimeout,
	})
	kernel.GET("/boom", func(ctx *genhttp.Context) error {
		return contracts.NewHTTPError(500, "handler exploded")
	})

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	served := make(chan error, 1)
	go func() { served <- kernel.Fiber().Listener(listener) }()

	t.Cleanup(func() {
		require.NoError(t, kernel.Shutdown())
		select {
		case <-served:
		case <-time.After(5 * time.Second):
			t.Error("the server did not stop")
		}
	})

	return listener.Addr().String(), logs
}

// A browser that opens a spare socket and never sends anything on it -
// Chrome and Safari both preconnect speculatively - has not made a
// request, so its read timeout is not an application error. Reporting it
// as one fills the log with failures against a path and method that were
// never sent.
func TestConnectionThatSendsNothingIsNotReportedAsAnError(t *testing.T) {
	addr, logs := serveForConnectionTests(t, 200*time.Millisecond)

	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	require.NoError(t, err)
	defer conn.Close()

	// Nothing is written. The deadline is the safety net: if the server
	// stops answering these connections at all the test fails here
	// rather than hanging.
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(5*time.Second)))
	_, err = io.ReadAll(conn)
	require.NoError(t, err, "the server never closed the timed-out connection")

	for _, record := range logs.recordsAt(t, "error") {
		t.Errorf("a connection that sent nothing was reported at error level: %v", record)
	}

	var reported map[string]any
	for _, record := range logs.recordsAt(t, "debug") {
		if record["message"] == "Connection closed before a request arrived" {
			reported = record
		}
	}
	require.NotNil(t, reported, "the connection failure should still be findable at debug level: %v", logs.records(t))
	assert.NotContains(t, reported, "path", "no path was ever received, so none may be reported")
	assert.NotContains(t, reported, "method", "no method was ever received, so none may be reported")
	assert.Equal(t, "127.0.0.1", reported["ip"], "the connecting peer is the one thing that is known")
}

// A request that did arrive and whose handler failed is a real error and
// stays one, reported against the path and method the client actually
// sent.
func TestHandlerErrorIsStillReportedWithItsPathAndMethod(t *testing.T) {
	addr, logs := serveForConnectionTests(t, 5*time.Second)

	resp := get(t, addr, "/boom")
	assert.Equal(t, 500, resp)

	errorRecords := logs.recordsAt(t, "error")
	require.Len(t, errorRecords, 1, "the handler failure should be reported once: %v", logs.records(t))
	assert.Equal(t, "Error occurred", errorRecords[0]["message"])
	assert.Equal(t, "/boom", errorRecords[0]["path"])
	assert.Equal(t, "GET", errorRecords[0]["method"])
}

// A 404 is a request that arrived, was understood and was answered, so
// it keeps the error report it has always had. The connection check
// deliberately does not cover it: whether it should be quieter is a
// separate question from whether a connection failure is an error.
func TestNotFoundIsStillReportedWithItsPathAndMethod(t *testing.T) {
	addr, logs := serveForConnectionTests(t, 5*time.Second)

	resp := get(t, addr, "/no-such-route")
	assert.Equal(t, 404, resp)

	errorRecords := logs.recordsAt(t, "error")
	require.Len(t, errorRecords, 1, "the 404 should be reported once: %v", logs.records(t))
	assert.Equal(t, "Error occurred", errorRecords[0]["message"])
	assert.Equal(t, "/no-such-route", errorRecords[0]["path"])
	assert.Equal(t, "GET", errorRecords[0]["method"])
}

// get performs one request against the test server and returns its
// status, with deadlines throughout so a regression fails rather than
// hangs.
func get(t *testing.T, addr, path string) int {
	t.Helper()

	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	require.NoError(t, err)
	defer conn.Close()
	require.NoError(t, conn.SetDeadline(time.Now().Add(5*time.Second)))

	_, err = conn.Write([]byte("GET " + path + " HTTP/1.1\r\nHost: " + addr + "\r\nConnection: close\r\n\r\n"))
	require.NoError(t, err)

	request, err := nethttp.NewRequest(nethttp.MethodGet, "http://"+addr+path, nil)
	require.NoError(t, err)
	resp, err := nethttp.ReadResponse(bufio.NewReader(conn), request)
	require.NoError(t, err)
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	return resp.StatusCode
}
