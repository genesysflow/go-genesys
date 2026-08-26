// Package testutil provides shared test utilities for the go-genesys framework.
package testutil

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/rand/v2"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	// The host readiness probe talks to the container over TCP; the driver
	// is a database driver, not a container runtime, so the framework still
	// carries no container-runtime dependency. Importing the framework's
	// own database package here would create an import cycle with its tests.
	_ "github.com/lib/pq"
)

const (
	// minHostPort is the bottom of the range we publish containers on.
	minHostPort = 20000
	// defaultEphemeralLow is the Linux default lower bound of the ephemeral
	// port range, used when the kernel setting cannot be read (macOS).
	defaultEphemeralLow = 32768
	// hostPortCandidates bounds how many ports freeHostPort tries.
	hostPortCandidates = 20
	// maxContainerAttempts bounds how many containers a single setup starts.
	maxContainerAttempts = 3
	// readyTimeout bounds how long we wait for a container to answer queries.
	readyTimeout = 60 * time.Second
	// probeTimeout bounds a single probe so a hung connect cannot eat the
	// whole readiness deadline.
	probeTimeout = 2 * time.Second
	// execProbeTimeout bounds a single `docker exec` probe.
	execProbeTimeout = 10 * time.Second
)

// Indirections so tests can drive the retry logic without Docker.
var (
	dockerCmd = docker
	hostProbe = queryFromHost
	execProbe = queryInContainer

	// pollInterval is the delay between readiness probes.
	pollInterval = 250 * time.Millisecond
	// execProbeInterval bounds how often the (comparatively expensive)
	// in-container probe runs while the host probe keeps failing.
	execProbeInterval = time.Second
)

// errPortUnusable marks a failure caused by the published host port rather
// than by postgres: docker refused to bind it, or it bound it and the
// forward never carried traffic. Retrying on a different port is worthwhile.
var errPortUnusable = errors.New("host port unusable")

// PostgresContainer describes a disposable PostgreSQL container started
// for integration tests. It is driven through the docker CLI directly,
// so the framework carries no container-runtime library dependencies.
type PostgresContainer struct {
	// ID is the docker container id.
	ID       string
	Host     string
	Port     int
	Database string
	Username string
	Password string
}

// dockerAvailable reports whether a Docker daemon is reachable.
func dockerAvailable() bool {
	if _, err := exec.LookPath("docker"); err != nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, "docker", "info").Run() == nil
}

// docker runs a docker CLI command and returns its trimmed stdout.
func docker(ctx context.Context, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("docker %s: %w (%s)", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

// parsePortRange parses the contents of /proc/sys/net/ipv4/ip_local_port_range,
// two whitespace separated ports, e.g. "32768\t60999\n".
func parsePortRange(contents string) (low, high int, err error) {
	fields := strings.Fields(contents)
	if len(fields) != 2 {
		return 0, 0, fmt.Errorf("unexpected local port range %q", contents)
	}
	if low, err = strconv.Atoi(fields[0]); err != nil {
		return 0, 0, fmt.Errorf("unexpected local port range %q: %w", contents, err)
	}
	if high, err = strconv.Atoi(fields[1]); err != nil {
		return 0, 0, fmt.Errorf("unexpected local port range %q: %w", contents, err)
	}
	if low < 1 || high > 65535 || low >= high {
		return 0, 0, fmt.Errorf("nonsensical local port range %q", contents)
	}
	return low, high, nil
}

// ephemeralLow returns the lowest port the kernel hands out as an outgoing
// socket's source port. Publishing a container above it is a hazard: docker
// allocates from 32768 up, inside that range, and when a local socket
// already holds the port as a source port the forward's bind fails
// silently. The container is healthy and `docker port` reports the mapping,
// yet every host connection is refused for the container's whole lifetime.
func ephemeralLow() int {
	contents, err := os.ReadFile("/proc/sys/net/ipv4/ip_local_port_range")
	if err != nil {
		// Not Linux (macOS): assume the common default.
		return defaultEphemeralLow
	}
	low, _, err := parsePortRange(string(contents))
	if err != nil || low <= minHostPort {
		return defaultEphemeralLow
	}
	return low
}

// freeHostPort picks a currently free loopback port below the ephemeral
// range, so nothing else on the machine can be using it as a source port
// when docker binds the published port.
func freeHostPort() (int, error) {
	high := ephemeralLow()

	for range hostPortCandidates {
		port := minHostPort + rand.IntN(high-minHostPort)
		listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
		if err != nil {
			continue
		}
		if err := listener.Close(); err != nil {
			continue
		}
		return port, nil
	}
	return 0, fmt.Errorf("no free host port in [%d, %d) after %d attempts", minHostPort, high, hostPortCandidates)
}

// isConnectionRefused reports whether err is a refused TCP connection.
// lib/pq returns the *net.OpError from the failed dial unchanged, so
// errors.Is walks the OpError -> os.SyscallError -> syscall.Errno chain;
// the string check is a last resort for drivers that stringify the cause.
func isConnectionRefused(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, syscall.ECONNREFUSED) {
		return true
	}
	return strings.Contains(err.Error(), "connection refused")
}

// portTakenMarkers are the phrases docker uses when it cannot publish the
// requested port. The wording differs per backend: the Linux engine says
// "port is already allocated" / "address already in use", Docker Desktop's
// VM proxy says "ports are not available: exposing port TCP 127.0.0.1:P".
var portTakenMarkers = []string{
	"address already in use",
	"port is already allocated",
	"ports are not available",
	"failed programming external connectivity",
	"bind for 127.0.0.1",
}

// isPortTaken reports whether a `docker run` failure was caused by the
// published port already being in use.
func isPortTaken(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	for _, marker := range portTakenMarkers {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

// queryFromHost runs a real query through the published port, the same path
// the tests take.
func queryFromHost(ctx context.Context, pc *PostgresContainer) error {
	db, err := sql.Open("postgres", pc.DSN())
	if err != nil {
		return err
	}
	defer db.Close()

	attemptCtx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	var one int
	return db.QueryRowContext(attemptCtx, "SELECT 1").Scan(&one)
}

// queryInContainer runs the same query from inside the container, over the
// container's own loopback, bypassing the published port entirely.
func queryInContainer(ctx context.Context, pc *PostgresContainer) error {
	attemptCtx, cancel := context.WithTimeout(ctx, execProbeTimeout)
	defer cancel()

	_, err := dockerCmd(attemptCtx, "exec", "-e", "PGPASSWORD="+pc.Password, pc.ID,
		"psql", "-h", "127.0.0.1", "-U", pc.Username, "-d", pc.Database, "-c", "SELECT 1")
	return err
}

// SetupPostgresContainer creates a PostgreSQL container for integration
// testing. It returns the container info and a cleanup function that
// should be deferred. The test is skipped when no Docker daemon is
// available.
func SetupPostgresContainer(t *testing.T) (*PostgresContainer, func()) {
	t.Helper()

	if !dockerAvailable() {
		t.Skip("skipping: Docker is not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	return setupPostgresContainer(ctx, t)
}

// setupPostgresContainer starts containers until one is reachable from the
// host, giving up after maxContainerAttempts.
func setupPostgresContainer(ctx context.Context, t *testing.T) (*PostgresContainer, func()) {
	t.Helper()

	var lastErr error
	for attempt := 1; attempt <= maxContainerAttempts; attempt++ {
		pc, cleanup, err := startPostgresContainer(ctx, t)
		if err == nil {
			return pc, cleanup
		}
		if cleanup != nil {
			cleanup()
		}
		lastErr = err
		if !errors.Is(err, errPortUnusable) {
			t.Fatalf("%v", err)
		}
		t.Logf("postgres container attempt %d/%d failed, retrying on a new port: %v",
			attempt, maxContainerAttempts, err)
	}
	t.Fatalf("postgres container unusable after %d attempts: %v", maxContainerAttempts, lastErr)
	return nil, nil
}

// startPostgresContainer starts one container on a port we choose ourselves
// and waits until the host can query it. The returned cleanup is non-nil
// whenever a container was created, even alongside an error.
func startPostgresContainer(ctx context.Context, t *testing.T) (*PostgresContainer, func(), error) {
	t.Helper()

	port, err := freeHostPort()
	if err != nil {
		return nil, nil, err
	}

	id, err := dockerCmd(ctx, "run", "-d", "--rm",
		"-e", "POSTGRES_DB=testdb",
		"-e", "POSTGRES_USER=testuser",
		"-e", "POSTGRES_PASSWORD=testpass",
		"-p", fmt.Sprintf("127.0.0.1:%d:5432", port),
		"postgres:16-alpine",
	)
	if err != nil {
		if isPortTaken(err) {
			return nil, nil, fmt.Errorf("%w: %v", errPortUnusable, err)
		}
		return nil, nil, fmt.Errorf("failed to start postgres container: %w", err)
	}

	pc := &PostgresContainer{
		ID:       id,
		Host:     "127.0.0.1",
		Port:     port,
		Database: "testdb",
		Username: "testuser",
		Password: "testpass",
	}

	cleanup := func() {
		removeCtx, removeCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer removeCancel()
		if _, err := dockerCmd(removeCtx, "rm", "-f", pc.ID); err != nil {
			t.Logf("failed to remove container: %v", err)
		}
	}

	if err := confirmPublishedPort(ctx, pc); err != nil {
		return pc, cleanup, err
	}
	if err := waitForHostQuery(ctx, pc); err != nil {
		return pc, cleanup, err
	}
	return pc, cleanup, nil
}

// confirmPublishedPort checks docker published 5432 on the port we asked for.
func confirmPublishedPort(ctx context.Context, pc *PostgresContainer) error {
	mapping, err := dockerCmd(ctx, "port", pc.ID, "5432/tcp")
	if err != nil {
		return fmt.Errorf("failed to get mapped port: %w", err)
	}
	// The first line looks like "127.0.0.1:20345".
	firstLine := strings.TrimSpace(strings.SplitN(mapping, "\n", 2)[0])
	colon := strings.LastIndex(firstLine, ":")
	if colon < 0 {
		return fmt.Errorf("unexpected docker port output: %q", mapping)
	}
	published, err := strconv.Atoi(strings.TrimSpace(firstLine[colon+1:]))
	if err != nil {
		return fmt.Errorf("unexpected docker port %q: %w", firstLine[colon+1:], err)
	}
	if published != pc.Port {
		return fmt.Errorf("docker published port %d, expected %d", published, pc.Port)
	}
	return nil
}

// waitForHostQuery waits for the FINAL server to answer a real query over
// TCP from the host, through the published port. Both halves matter. The
// postgres image's entrypoint first runs a temporary, unix-socket-only
// server while it initializes, then restarts; pg_isready succeeds against
// that temporary server, letting tests connect from the host in the window
// where TCP is still refused ("connection reset by peer"), and a query only
// succeeds once the final server is up. Probing from the host rather than
// from inside the container additionally proves docker's port forward
// carries traffic, which an in-container probe cannot see.
//
// When the host is refused while postgres answers inside the container, the
// forward is dead for this container's lifetime (its bind lost a race with
// a local socket holding the same port); the caller replaces the container.
func waitForHostQuery(ctx context.Context, pc *PostgresContainer) error {
	deadline := time.Now().Add(readyTimeout)
	nextExecProbe := time.Now().Add(execProbeInterval)

	var lastErr error
	for time.Now().Before(deadline) {
		err := hostProbe(ctx, pc)
		if err == nil {
			return nil
		}
		lastErr = err

		if isConnectionRefused(err) && !time.Now().Before(nextExecProbe) {
			nextExecProbe = time.Now().Add(execProbeInterval)
			if execProbe(ctx, pc) == nil {
				// Postgres is listening on TCP inside the container.
				// Re-probe the host to rule out having caught the
				// instant the final server came up.
				confirm := hostProbe(ctx, pc)
				if confirm == nil {
					return nil
				}
				if isConnectionRefused(confirm) {
					return fmt.Errorf("%w: container ready but host port %d unreachable"+
						" — published port likely collided with a local socket", errPortUnusable, pc.Port)
				}
				lastErr = confirm
			}
		}
		time.Sleep(pollInterval)
	}
	return fmt.Errorf("postgres container did not become ready in time: %w", lastErr)
}

// DSN returns the PostgreSQL connection string for the test container.
func (pc *PostgresContainer) DSN() string {
	return "host=" + pc.Host +
		" port=" + strconv.Itoa(pc.Port) +
		" user=" + pc.Username +
		" password=" + pc.Password +
		" dbname=" + pc.Database +
		" sslmode=disable"
}

// PostgresForTests returns PostgreSQL connection info for integration
// tests: from GENESYS_TEST_PG_* environment variables when set (a CI
// services container), otherwise from a docker container started on the
// fly. The test is skipped when neither is available.
func PostgresForTests(t *testing.T) *PostgresContainer {
	t.Helper()

	if host := os.Getenv("GENESYS_TEST_PG_HOST"); host != "" {
		port := 5432
		if raw := os.Getenv("GENESYS_TEST_PG_PORT"); raw != "" {
			if parsed, err := strconv.Atoi(raw); err == nil {
				port = parsed
			}
		}
		pc := &PostgresContainer{
			Host:     host,
			Port:     port,
			Database: envOr("GENESYS_TEST_PG_DB", "testdb"),
			Username: envOr("GENESYS_TEST_PG_USER", "testuser"),
			Password: envOr("GENESYS_TEST_PG_PASSWORD", "testpass"),
		}
		return pc
	}

	pc, cleanup := SetupPostgresContainer(t)
	t.Cleanup(cleanup)
	return pc
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
