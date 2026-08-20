// Package testutil provides shared test utilities for the go-genesys framework.
package testutil

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// PostgresContainer holds the container and cleanup function for tests.
type PostgresContainer struct {
	Container testcontainers.Container
	Host      string
	Port      int
	Database  string
	Username  string
	Password  string
}

// dockerAvailable reports whether a Docker daemon is reachable for
// testcontainers. Provider discovery panics on some hosts (e.g. rootless
// Docker lookups), so treat any panic as "not available".
func dockerAvailable() (ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	provider, err := testcontainers.NewDockerProvider()
	if err != nil {
		return false
	}
	defer provider.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return provider.Health(ctx) == nil
}

// SetupPostgresContainer creates a PostgreSQL container for integration testing.
// It returns the container info and a cleanup function that should be deferred.
// The test is skipped when no Docker daemon is available.
func SetupPostgresContainer(t *testing.T) (*PostgresContainer, func()) {
	t.Helper()

	if !dockerAvailable() {
		t.Skip("skipping: Docker is not available for testcontainers")
	}

	ctx := context.Background()

	container, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("testuser"),
		postgres.WithPassword("testpass"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("failed to start postgres container: %v", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		container.Terminate(ctx)
		t.Fatalf("failed to get container host: %v", err)
	}

	port, err := container.MappedPort(ctx, "5432")
	if err != nil {
		container.Terminate(ctx)
		t.Fatalf("failed to get mapped port: %v", err)
	}

	pc := &PostgresContainer{
		Container: container,
		Host:      host,
		Port:      port.Int(),
		Database:  "testdb",
		Username:  "testuser",
		Password:  "testpass",
	}

	cleanup := func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf("failed to terminate container: %v", err)
		}
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
