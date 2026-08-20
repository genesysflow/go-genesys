// Package testutil provides shared test utilities for the go-genesys framework.
package testutil

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

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

	id, err := docker(ctx, "run", "-d", "--rm",
		"-e", "POSTGRES_DB=testdb",
		"-e", "POSTGRES_USER=testuser",
		"-e", "POSTGRES_PASSWORD=testpass",
		"-p", "127.0.0.1:0:5432",
		"postgres:16-alpine",
	)
	if err != nil {
		t.Fatalf("failed to start postgres container: %v", err)
	}

	cleanup := func() {
		removeCtx, removeCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer removeCancel()
		if _, err := docker(removeCtx, "rm", "-f", id); err != nil {
			t.Logf("failed to remove container: %v", err)
		}
	}

	fail := func(format string, args ...any) {
		cleanup()
		t.Fatalf(format, args...)
	}

	// Resolve the host port docker mapped for 5432.
	mapping, err := docker(ctx, "port", id, "5432/tcp")
	if err != nil {
		fail("failed to get mapped port: %v", err)
	}
	// First line looks like "127.0.0.1:49153".
	firstLine := strings.SplitN(mapping, "\n", 2)[0]
	host, portText, found := strings.Cut(firstLine, ":")
	if !found {
		fail("unexpected docker port output: %q", mapping)
	}
	port, err := strconv.Atoi(strings.TrimSpace(portText))
	if err != nil {
		fail("unexpected docker port %q: %v", portText, err)
	}

	// Wait for postgres to accept connections.
	ready := false
	for deadline := time.Now().Add(60 * time.Second); time.Now().Before(deadline); {
		if _, err := docker(ctx, "exec", id, "pg_isready", "-U", "testuser", "-d", "testdb"); err == nil {
			ready = true
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if !ready {
		fail("postgres container did not become ready in time")
	}

	pc := &PostgresContainer{
		ID:       id,
		Host:     host,
		Port:     port,
		Database: "testdb",
		Username: "testuser",
		Password: "testpass",
	}
	return pc, cleanup
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
