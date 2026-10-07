package database

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// flakyDriver fails its first `failures` opens (a negative count fails
// every open) with a dial-style error, then hands out working
// connections. The error is a plain one, not driver.ErrBadConn, which
// database/sql would retry on its own and throw off the attempt count.
type flakyDriver struct {
	failures int64
	opens    atomic.Int64
}

var errRefused = errors.New("dial tcp 10.43.0.10:5432: connect: connection refused")

func (d *flakyDriver) Open(string) (driver.Conn, error) {
	n := d.opens.Add(1)
	if d.failures < 0 || n <= d.failures {
		return nil, errRefused
	}
	return flakyConn{}, nil
}

type flakyConn struct{}

func (flakyConn) Prepare(string) (driver.Stmt, error) { return nil, errors.New("not supported") }
func (flakyConn) Close() error                        { return nil }
func (flakyConn) Begin() (driver.Tx, error)           { return nil, errors.New("not supported") }

var flakyDriverSeq atomic.Int64

// registerFlakyDriver registers a fresh driver under a unique name;
// sql.Register panics on a reused name, which -count=N would hit.
func registerFlakyDriver(failures int64) (string, *flakyDriver) {
	d := &flakyDriver{failures: failures}
	name := fmt.Sprintf("flaky-test-%d", flakyDriverSeq.Add(1))
	sql.Register(name, d)
	return name, d
}

// fastConnectRetry shrinks the retry timings for the test.
func fastConnectRetry(t *testing.T) {
	t.Helper()
	prevDefault, prevInitial, prevMax := defaultConnectRetry, connectRetryInitialBackoff, connectRetryMaxBackoff
	prevTimeout, prevMin := connectPingTimeout, connectPingMinTimeout
	defaultConnectRetry = 2 * time.Second
	connectRetryInitialBackoff = time.Millisecond
	connectRetryMaxBackoff = 4 * time.Millisecond
	connectPingTimeout = time.Second
	connectPingMinTimeout = 10 * time.Millisecond
	t.Cleanup(func() {
		defaultConnectRetry, connectRetryInitialBackoff, connectRetryMaxBackoff = prevDefault, prevInitial, prevMax
		connectPingTimeout, connectPingMinTimeout = prevTimeout, prevMin
	})
}

func flakyManager(driverName string, retry time.Duration) *Manager {
	return NewManager(Config{
		Default: "default",
		Connections: map[string]ConnectionConfig{
			"default": {Driver: driverName, ConnectRetry: retry},
		},
	})
}

// A database that refuses the first few connections - a new pod whose
// NetworkPolicy has not caught up yet - is waited for, not failed.
func TestConnectionRetriesUntilTheDatabaseAnswers(t *testing.T) {
	fastConnectRetry(t)
	name, d := registerFlakyDriver(3)

	m := flakyManager(name, 0) // default budget
	t.Cleanup(func() { _ = m.Close() })

	conn := m.Connection()
	require.NoError(t, conn.Error())
	assert.Equal(t, int64(4), d.opens.Load(), "three refusals, then the attempt that succeeded")
	require.NoError(t, conn.Ping())
}

// When the budget runs out, the error names the attempts and carries
// the last ping error, so callers see why rather than just that.
func TestConnectionGivesUpWithTheRealCause(t *testing.T) {
	fastConnectRetry(t)
	name, d := registerFlakyDriver(-1)

	m := flakyManager(name, 30*time.Millisecond)
	t.Cleanup(func() { _ = m.Close() })

	start := time.Now()
	conn := m.Connection()
	elapsed := time.Since(start)

	err := conn.Error()
	require.Error(t, err)
	assert.ErrorIs(t, err, errRefused)
	assert.Contains(t, err.Error(), "connect: connection refused")
	attempts := d.opens.Load()
	assert.Greater(t, attempts, int64(1))
	assert.Contains(t, err.Error(), fmt.Sprintf("(%d attempts)", attempts))
	assert.Less(t, elapsed, time.Second, "the configured budget bounds the wait")

	// The failed connection surfaces the same cause from every method.
	assert.ErrorIs(t, conn.PingContext(context.Background()), errRefused)
}

// A negative ConnectRetry turns retries off: one ping, then the error.
func TestConnectionRetryCanBeDisabled(t *testing.T) {
	fastConnectRetry(t)
	name, d := registerFlakyDriver(-1)

	m := flakyManager(name, -1)
	t.Cleanup(func() { _ = m.Close() })

	err := m.Connection().Error()
	require.Error(t, err)
	assert.ErrorIs(t, err, errRefused)
	assert.Equal(t, int64(1), d.opens.Load())
	assert.Equal(t, "failed to ping database: "+errRefused.Error(), err.Error())
}

// An error waiting cannot fix - here an unknown driver - fails at once.
func TestConnectionDoesNotRetryAnUnknownDriver(t *testing.T) {
	m := flakyManager("no-such-driver-for-retry", 0)
	t.Cleanup(func() { _ = m.Close() })

	start := time.Now()
	err := m.Connection().Error()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to open database")
	assert.Less(t, time.Since(start), 100*time.Millisecond)
}
