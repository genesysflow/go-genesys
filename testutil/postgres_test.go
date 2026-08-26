package testutil

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

func TestParsePortRange(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		low     int
		high    int
		wantErr bool
	}{
		{name: "linux default", input: "32768\t60999\n", low: 32768, high: 60999},
		{name: "space separated", input: "1024 65535", low: 1024, high: 65535},
		{name: "extra whitespace", input: "  32768   60999  \n", low: 32768, high: 60999},
		{name: "garbage", input: "not a range", wantErr: true},
		{name: "empty", input: "", wantErr: true},
		{name: "single value", input: "32768", wantErr: true},
		{name: "non numeric low", input: "abc 60999", wantErr: true},
		{name: "non numeric high", input: "32768 abc", wantErr: true},
		{name: "inverted", input: "60999 32768", wantErr: true},
		{name: "out of range", input: "32768 70000", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			low, high, err := parsePortRange(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parsePortRange(%q) = (%d, %d, nil), want an error", tc.input, low, high)
				}
				return
			}
			if err != nil {
				t.Fatalf("parsePortRange(%q) returned error: %v", tc.input, err)
			}
			if low != tc.low || high != tc.high {
				t.Errorf("parsePortRange(%q) = (%d, %d), want (%d, %d)", tc.input, low, high, tc.low, tc.high)
			}
		})
	}
}

func TestEphemeralLowIsAboveMinHostPort(t *testing.T) {
	if got := ephemeralLow(); got <= minHostPort {
		t.Fatalf("ephemeralLow() = %d, want a value above minHostPort (%d)", got, minHostPort)
	}
}

func TestFreeHostPortAvoidsEphemeralRange(t *testing.T) {
	low := ephemeralLow()

	for range 10 {
		port, err := freeHostPort()
		if err != nil {
			t.Fatalf("freeHostPort() returned error: %v", err)
		}
		if port < minHostPort || port >= low {
			t.Fatalf("freeHostPort() = %d, want a port in [%d, %d)", port, minHostPort, low)
		}

		listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
		if err != nil {
			t.Fatalf("port %d reported free but is not listenable: %v", port, err)
		}
		listener.Close()
	}
}

func TestFreeHostPortSkipsBusyPorts(t *testing.T) {
	// A port held by a listener must never be handed out.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer listener.Close()
	busy := listener.Addr().(*net.TCPAddr).Port

	for range 50 {
		port, err := freeHostPort()
		if err != nil {
			t.Fatalf("freeHostPort() returned error: %v", err)
		}
		if port == busy {
			t.Fatalf("freeHostPort() handed out busy port %d", port)
		}
	}
}

func TestIsConnectionRefused(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	conn, dialErr := net.Dial("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if dialErr == nil {
		conn.Close()
		t.Skip("skipping: the closed port was reused before the dial")
	}
	if !isConnectionRefused(dialErr) {
		t.Errorf("isConnectionRefused(%v) = false, want true", dialErr)
	}

	if isConnectionRefused(nil) {
		t.Error("isConnectionRefused(nil) = true, want false")
	}
	if isConnectionRefused(errors.New("pq: password authentication failed")) {
		t.Error("isConnectionRefused(auth error) = true, want false")
	}
	if isConnectionRefused(context.DeadlineExceeded) {
		t.Error("isConnectionRefused(context.DeadlineExceeded) = true, want false")
	}
}

func TestIsPortTaken(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "linux engine",
			err: errors.New("docker run: exit status 125 (docker: Error response from daemon: " +
				"driver failed programming external connectivity on endpoint gifted_bell: " +
				"Bind for 127.0.0.1:20345 failed: port is already allocated)"),
			want: true,
		},
		{
			// Verbatim from Docker Desktop on WSL2.
			name: "docker desktop",
			err: errors.New("docker run: exit status 125 (docker: Error response from daemon: " +
				"ports are not available: exposing port TCP 127.0.0.1:22381 -> 127.0.0.1:0: " +
				"/forwards/expose returned unexpected status: 500)"),
			want: true,
		},
		{
			name: "rootless listen",
			err:  errors.New("docker run: exit status 125 (listen tcp 127.0.0.1:20345: bind: address already in use)"),
			want: true,
		},
		{name: "nil", err: nil, want: false},
		{
			name: "image pull failure",
			err:  errors.New("docker run: exit status 125 (Error response from daemon: manifest unknown)"),
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isPortTaken(tc.err); got != tc.want {
				t.Errorf("isPortTaken(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

// refusedError mirrors what the net package returns for a refused dial,
// which is exactly what lib/pq hands back from a failed connect.
func refusedError(port int) error {
	return &net.OpError{
		Op:   "dial",
		Net:  "tcp",
		Addr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: port},
		Err:  os.NewSyscallError("connect", syscall.ECONNREFUSED),
	}
}

// stubDocker swaps the docker CLI and both readiness probes for the
// duration of a test.
func stubDocker(t *testing.T, run func(ctx context.Context, args ...string) (string, error)) {
	t.Helper()

	originalDocker, originalHost, originalExec := dockerCmd, hostProbe, execProbe
	originalExecInterval, originalPoll := execProbeInterval, pollInterval
	t.Cleanup(func() {
		dockerCmd, hostProbe, execProbe = originalDocker, originalHost, originalExec
		execProbeInterval, pollInterval = originalExecInterval, originalPoll
	})

	dockerCmd = run
	execProbeInterval = 0
	pollInterval = time.Millisecond
}

// publishedPort extracts the host port from a docker run argument list.
func publishedPort(t *testing.T, args []string) int {
	t.Helper()

	for i, arg := range args {
		if arg != "-p" || i+1 >= len(args) {
			continue
		}
		parts := strings.Split(args[i+1], ":")
		if len(parts) != 3 {
			t.Fatalf("unexpected -p argument %q", args[i+1])
		}
		port, err := strconv.Atoi(parts[1])
		if err != nil {
			t.Fatalf("unexpected -p argument %q: %v", args[i+1], err)
		}
		return port
	}
	t.Fatalf("no -p argument in %v", args)
	return 0
}

func TestSetupPostgresContainerReplacesUnreachableContainer(t *testing.T) {
	var (
		mu      sync.Mutex
		runs    []int
		removed []string
	)

	stubDocker(t, func(ctx context.Context, args ...string) (string, error) {
		mu.Lock()
		defer mu.Unlock()

		switch args[0] {
		case "run":
			port := publishedPort(t, args)
			runs = append(runs, port)
			return fmt.Sprintf("container-%d", len(runs)), nil
		case "port":
			index, err := strconv.Atoi(strings.TrimPrefix(args[1], "container-"))
			if err != nil {
				return "", fmt.Errorf("unknown container %q", args[1])
			}
			return fmt.Sprintf("127.0.0.1:%d", runs[index-1]), nil
		case "rm":
			removed = append(removed, args[len(args)-1])
			return "", nil
		}
		return "", fmt.Errorf("unexpected docker command %v", args)
	})

	// The first container publishes a dead port: the host is refused
	// while postgres answers happily inside the container.
	hostProbe = func(ctx context.Context, pc *PostgresContainer) error {
		if pc.ID == "container-1" {
			return refusedError(pc.Port)
		}
		return nil
	}
	execProbe = func(ctx context.Context, pc *PostgresContainer) error { return nil }

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pc, cleanup := setupPostgresContainer(ctx, t)
	defer cleanup()

	if pc.ID != "container-2" {
		t.Errorf("container id = %q, want the replacement container-2", pc.ID)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(runs) != 2 {
		t.Fatalf("docker run called %d times, want 2", len(runs))
	}
	if runs[0] == runs[1] {
		t.Errorf("replacement reused host port %d, want a fresh one", runs[0])
	}
	if len(removed) == 0 || removed[0] != "container-1" {
		t.Errorf("removed = %v, want the unreachable container-1 removed first", removed)
	}
	if pc.Port != runs[1] {
		t.Errorf("pc.Port = %d, want the published port %d", pc.Port, runs[1])
	}
}

func TestSetupPostgresContainerRetriesWhenPortIsAllocated(t *testing.T) {
	var (
		runs     int
		lastPort int
	)

	stubDocker(t, func(ctx context.Context, args ...string) (string, error) {
		switch args[0] {
		case "run":
			runs++
			lastPort = publishedPort(t, args)
			if runs == 1 {
				return "", fmt.Errorf("docker run: exit status 125 (docker: Error response from daemon: "+
					"driver failed programming external connectivity on endpoint: Bind for 127.0.0.1:%d failed: "+
					"port is already allocated)", lastPort)
			}
			return "container-ok", nil
		case "port":
			return "127.0.0.1:" + strconv.Itoa(lastPort), nil
		case "rm":
			return "", nil
		}
		return "", fmt.Errorf("unexpected docker command %v", args)
	})

	hostProbe = func(ctx context.Context, pc *PostgresContainer) error { return nil }
	execProbe = func(ctx context.Context, pc *PostgresContainer) error { return nil }

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pc, cleanup := setupPostgresContainer(ctx, t)
	defer cleanup()

	if runs != 2 {
		t.Errorf("docker run called %d times, want 2 (one collision, one success)", runs)
	}
	if pc.ID != "container-ok" {
		t.Errorf("container id = %q, want container-ok", pc.ID)
	}
}

func TestSetupPostgresContainerIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	if !dockerAvailable() {
		t.Skip("skipping: Docker is not available")
	}

	pc, cleanup := SetupPostgresContainer(t)
	defer cleanup()

	if pc.Port < minHostPort || pc.Port >= ephemeralLow() {
		t.Errorf("published port %d is inside the ephemeral range, want [%d, %d)",
			pc.Port, minHostPort, ephemeralLow())
	}

	// The container must be queryable from the host the moment setup returns.
	db, err := sql.Open("postgres", pc.DSN())
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var one int
	if err := db.QueryRowContext(ctx, "SELECT 1").Scan(&one); err != nil {
		t.Fatalf("failed to query container through %s: %v", pc.DSN(), err)
	}
	if one != 1 {
		t.Errorf("SELECT 1 returned %d", one)
	}
}

// TestSetupPostgresContainerSurvivesPublishedPortCollision reproduces the
// flake this package's port handling exists for. Docker publishes a host
// port, but on engines whose forwarder binds inside a different network
// namespace (Docker Desktop) that bind fails silently when a local socket
// already uses the port as its *source* port: the container is healthy,
// `docker port` reports the mapping, and the host is refused for the
// container's lifetime. Holding the port docker is about to publish as a
// connected socket's source port recreates that exactly; setup must still
// hand back a container the host can query.
func TestSetupPostgresContainerSurvivesPublishedPortCollision(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	if !dockerAvailable() {
		t.Skip("skipping: Docker is not available")
	}

	original := dockerCmd
	t.Cleanup(func() { dockerCmd = original })

	// Something local to connect to, so the held port is a connected
	// socket's source port rather than a listener.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer listener.Close()

	var mu sync.Mutex
	var runPorts []int
	dockerCmd = func(ctx context.Context, args ...string) (string, error) {
		if len(args) > 0 && args[0] == "run" {
			port := publishedPort(t, args)
			mu.Lock()
			runPorts = append(runPorts, port)
			first := len(runPorts) == 1
			mu.Unlock()
			if first {
				dialer := net.Dialer{LocalAddr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: port}}
				held, err := dialer.DialContext(ctx, "tcp", listener.Addr().String())
				if err != nil {
					t.Fatalf("failed to occupy port %d as a source port: %v", port, err)
				}
				t.Cleanup(func() { held.Close() })
			}
		}
		return original(ctx, args...)
	}

	pc, cleanup := SetupPostgresContainer(t)
	defer cleanup()

	db, err := sql.Open("postgres", pc.DSN())
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var one int
	if err := db.QueryRowContext(ctx, "SELECT 1").Scan(&one); err != nil {
		t.Fatalf("container on port %d is not reachable from the host: %v", pc.Port, err)
	}

	mu.Lock()
	ports := append([]int(nil), runPorts...)
	mu.Unlock()
	if len(ports) >= 2 && pc.Port != ports[0] {
		t.Logf("collided container on port %d was replaced; docker runs on ports %v", ports[0], ports)
		return
	}
	// An engine whose forwarder shares the host's network namespace (the
	// Linux userland proxy binds with SO_REUSEADDR) may tolerate the
	// connected socket and publish the port anyway. Only Docker Desktop
	// is known to lose the bind, so only there is a replacement required.
	platform, _ := original(ctx, "version", "--format", "{{.Server.Platform.Name}}")
	if strings.Contains(platform, "Docker Desktop") {
		t.Fatalf("expected the container on held port %d to be replaced on %q; docker runs on ports %v",
			ports[0], platform, ports)
	}
	t.Logf("first container on held port %d was reachable on %q; replacement path not exercised", ports[0], platform)
}
