package filesystem

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/genesysflow/go-genesys/contracts"
)

// Mock config for testing
type mockConfig struct {
	data map[string]any
}

func (m *mockConfig) Get(key string) any {
	return m.data[key]
}

func (m *mockConfig) GetString(key string) string {
	if v, ok := m.data[key].(string); ok {
		return v
	}
	return ""
}

func (m *mockConfig) GetInt(key string) int {
	if v, ok := m.data[key].(int); ok {
		return v
	}
	return 0
}

func (m *mockConfig) GetBool(key string) bool {
	if v, ok := m.data[key].(bool); ok {
		return v
	}
	return false
}

func (m *mockConfig) GetFloat(key string) float64 {
	if v, ok := m.data[key].(float64); ok {
		return v
	}
	return 0
}

func (m *mockConfig) GetSlice(key string) []any {
	if v, ok := m.data[key].([]any); ok {
		return v
	}
	return nil
}

func (m *mockConfig) GetMap(key string) map[string]any {
	if v, ok := m.data[key].(map[string]any); ok {
		return v
	}
	return nil
}

func (m *mockConfig) GetStringSlice(key string) []string {
	if v, ok := m.data[key].([]string); ok {
		return v
	}
	return nil
}

func (m *mockConfig) GetStringMap(key string) map[string]string {
	if v, ok := m.data[key].(map[string]string); ok {
		return v
	}
	return nil
}

func (m *mockConfig) Has(key string) bool {
	_, ok := m.data[key]
	return ok
}

func (m *mockConfig) Set(key string, value any) {
	m.data[key] = value
}

func (m *mockConfig) All() map[string]any {
	return m.data
}

func (m *mockConfig) Load(path string) error {
	return nil
}

// Mock filesystem for testing
type mockFilesystem struct {
	name string
}

func (m *mockFilesystem) Exists(ctx context.Context, path string) bool {
	return false
}

func (m *mockFilesystem) Get(ctx context.Context, path string) (string, error) {
	return "", nil
}

func (m *mockFilesystem) GetBytes(ctx context.Context, path string) ([]byte, error) {
	return nil, nil
}

func (m *mockFilesystem) Put(ctx context.Context, path string, contents string) error {
	return nil
}

func (m *mockFilesystem) PutBytes(ctx context.Context, path string, contents []byte) error {
	return nil
}

func (m *mockFilesystem) PutStream(ctx context.Context, path string, contents io.Reader) error {
	return nil
}

func (m *mockFilesystem) Delete(ctx context.Context, path string) error {
	return nil
}

func (m *mockFilesystem) Copy(ctx context.Context, from, to string) error {
	return nil
}

func (m *mockFilesystem) Move(ctx context.Context, from, to string) error {
	return nil
}

func (m *mockFilesystem) Size(ctx context.Context, path string) (int64, error) {
	return 0, nil
}

func (m *mockFilesystem) LastModified(ctx context.Context, path string) (time.Time, error) {
	return time.Time{}, nil
}

func (m *mockFilesystem) MakeDirectory(ctx context.Context, path string) error {
	return nil
}

func (m *mockFilesystem) DeleteDirectory(ctx context.Context, path string) error {
	return nil
}

func (m *mockFilesystem) Url(path string) string {
	return ""
}

func setupManager(t *testing.T) (*Manager, *mockConfig) {
	t.Helper()

	tmpDir := t.TempDir()

	cfg := &mockConfig{
		data: map[string]any{
			"filesystem.default": "local",
			"filesystem.disks.local": map[string]any{
				"driver": "local",
				"root":   tmpDir,
				"url":    "http://localhost/storage",
			},
			"filesystem.disks.public": map[string]any{
				"driver": "local",
				"root":   tmpDir + "/public",
				"url":    "http://localhost/public",
			},
		},
	}

	manager := NewManager(cfg)
	return manager, cfg
}

func TestNewManager(t *testing.T) {
	cfg := &mockConfig{data: make(map[string]any)}
	manager := NewManager(cfg)

	if manager == nil {
		t.Fatal("expected manager instance, got nil")
	}
	if manager.config == nil {
		t.Error("expected config to be set")
	}
	if manager.disks == nil {
		t.Error("expected disks map to be initialized")
	}
	if manager.drivers == nil {
		t.Error("expected drivers map to be initialized")
	}
}

func TestManagerDisk(t *testing.T) {
	manager, _ := setupManager(t)

	t.Run("get default disk", func(t *testing.T) {
		disk := manager.Disk()
		if disk == nil {
			t.Fatal("expected disk instance, got nil")
		}

		// Verify it's cached
		disk2 := manager.Disk()
		if disk != disk2 {
			t.Error("expected same disk instance from cache")
		}
	})

	t.Run("get named disk", func(t *testing.T) {
		disk := manager.Disk("public")
		if disk == nil {
			t.Fatal("expected disk instance, got nil")
		}

		// Verify it's cached
		disk2 := manager.Disk("public")
		if disk != disk2 {
			t.Error("expected same disk instance from cache")
		}
	})

	t.Run("different disks are different instances", func(t *testing.T) {
		disk1 := manager.Disk("local")
		disk2 := manager.Disk("public")

		if disk1 == disk2 {
			t.Error("expected different disk instances for different names")
		}
	})
}

func TestManagerResolve(t *testing.T) {
	manager, cfg := setupManager(t)

	t.Run("resolve local driver", func(t *testing.T) {
		disk, err := manager.resolve("local")
		if err != nil {
			t.Fatalf("failed to resolve local driver: %v", err)
		}
		if disk == nil {
			t.Fatal("expected disk instance, got nil")
		}
	})

	t.Run("missing driver in config", func(t *testing.T) {
		cfg.data["filesystem.disks.nodrive"] = map[string]any{
			"root": "/tmp",
		}

		_, err := manager.resolve("nodrive")
		if err == nil {
			t.Fatal("expected error for missing driver")
		}
		if !strings.Contains(err.Error(), "driver not defined") {
			t.Errorf("expected 'driver not defined' error, got: %v", err)
		}
	})

	t.Run("unsupported driver", func(t *testing.T) {
		cfg.data["filesystem.disks.unsupported"] = map[string]any{
			"driver": "ftp",
			"host":   "example.com",
		}

		_, err := manager.resolve("unsupported")
		if err == nil {
			t.Fatal("expected error for unsupported driver")
		}
		if !strings.Contains(err.Error(), "not supported") {
			t.Errorf("expected 'not supported' error, got: %v", err)
		}
	})

	t.Run("nil config", func(t *testing.T) {
		_, err := manager.resolve("nonexistent")
		if err == nil {
			t.Fatal("expected error for nil config")
		}
	})
}

func TestManagerExtend(t *testing.T) {
	manager, cfg := setupManager(t)

	t.Run("register custom driver", func(t *testing.T) {
		called := false
		manager.Extend("custom", func(config map[string]any) (contracts.Filesystem, error) {
			called = true
			return &mockFilesystem{name: "custom"}, nil
		})

		cfg.data["filesystem.disks.custom"] = map[string]any{
			"driver": "custom",
		}

		disk, err := manager.resolve("custom")
		if err != nil {
			t.Fatalf("failed to resolve custom driver: %v", err)
		}
		if disk == nil {
			t.Fatal("expected disk instance, got nil")
		}
		if !called {
			t.Error("expected custom driver creator to be called")
		}
	})

	t.Run("custom driver via Disk method", func(t *testing.T) {
		manager.Extend("memory", func(config map[string]any) (contracts.Filesystem, error) {
			return &mockFilesystem{name: "memory"}, nil
		})

		cfg.data["filesystem.disks.cache"] = map[string]any{
			"driver": "memory",
		}

		disk := manager.Disk("cache")
		if disk == nil {
			t.Fatal("expected disk instance, got nil")
		}
	})

	t.Run("override built-in driver", func(t *testing.T) {
		overridden := false
		manager.Extend("local", func(config map[string]any) (contracts.Filesystem, error) {
			overridden = true
			return &mockFilesystem{name: "overridden-local"}, nil
		})

		cfg.data["filesystem.disks.override"] = map[string]any{
			"driver": "local",
			"root":   "/tmp/override",
		}

		_, err := manager.resolve("override")
		if err != nil {
			t.Fatalf("failed to resolve overridden driver: %v", err)
		}
		if !overridden {
			t.Error("expected custom local driver to override built-in")
		}
	})
}

func TestManagerGetDefaultDriver(t *testing.T) {
	cfg := &mockConfig{
		data: map[string]any{
			"filesystem.default": "s3",
		},
	}
	manager := NewManager(cfg)

	defaultDriver := manager.getDefaultDriver()
	if defaultDriver != "s3" {
		t.Errorf("expected default driver 's3', got '%s'", defaultDriver)
	}
}

func TestManagerGetConfig(t *testing.T) {
	cfg := &mockConfig{
		data: map[string]any{
			"filesystem.disks.test": map[string]any{
				"driver": "local",
				"root":   "/tmp",
			},
		},
	}
	manager := NewManager(cfg)

	t.Run("get existing config", func(t *testing.T) {
		config := manager.getConfig("test")
		if config == nil {
			t.Fatal("expected config map, got nil")
		}
		if config["driver"] != "local" {
			t.Errorf("expected driver 'local', got '%v'", config["driver"])
		}
		if config["root"] != "/tmp" {
			t.Errorf("expected root '/tmp', got '%v'", config["root"])
		}
	})

	t.Run("get non-existent config", func(t *testing.T) {
		config := manager.getConfig("nonexistent")
		if config != nil {
			t.Errorf("expected nil config for non-existent disk, got %v", config)
		}
	})

	t.Run("handle map[interface{}]interface{}", func(t *testing.T) {
		// Simulate YAML unmarshaling which sometimes produces map[interface{}]interface{}
		cfg.data["filesystem.disks.yaml"] = map[interface{}]interface{}{
			"driver": "local",
			"root":   "/yaml",
		}

		config := manager.getConfig("yaml")
		if config == nil {
			t.Fatal("expected config map, got nil")
		}
		if config["driver"] != "local" {
			t.Errorf("expected driver 'local', got '%v'", config["driver"])
		}
		if config["root"] != "/yaml" {
			t.Errorf("expected root '/yaml', got '%v'", config["root"])
		}
	})

	t.Run("invalid config type", func(t *testing.T) {
		cfg.data["filesystem.disks.invalid"] = "not a map"

		config := manager.getConfig("invalid")
		if config != nil {
			t.Errorf("expected nil config for invalid type, got %v", config)
		}
	})
}

func TestManagerThreadSafety(t *testing.T) {
	manager, cfg := setupManager(t)

	t.Run("concurrent disk access", func(t *testing.T) {
		var wg sync.WaitGroup
		diskNames := []string{"local", "public", "local", "public"}

		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				diskName := diskNames[idx%len(diskNames)]
				disk := manager.Disk(diskName)
				if disk == nil {
					t.Error("expected disk instance, got nil")
				}
			}(i)
		}

		wg.Wait()
	})

	t.Run("concurrent extend and resolve", func(t *testing.T) {
		var wg sync.WaitGroup

		// Register drivers concurrently
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				driverName := fmt.Sprintf("driver%d", idx)
				manager.Extend(driverName, func(config map[string]any) (contracts.Filesystem, error) {
					return &mockFilesystem{name: driverName}, nil
				})
			}(i)
		}

		wg.Wait()

		// Verify all drivers were registered
		if len(manager.drivers) < 10 {
			t.Errorf("expected at least 10 drivers, got %d", len(manager.drivers))
		}
	})

	t.Run("concurrent disk creation", func(t *testing.T) {
		var wg sync.WaitGroup
		results := make([]contracts.Filesystem, 50)

		// Pre-create all disk configurations
		for i := 0; i < 50; i++ {
			diskName := fmt.Sprintf("concurrent%d", i)
			cfg.data["filesystem.disks."+diskName] = map[string]any{
				"driver": "local",
				"root":   fmt.Sprintf("/tmp/test%d", i),
			}
		}

		// Create many disks concurrently
		for i := 0; i < 50; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				diskName := fmt.Sprintf("concurrent%d", idx)
				results[idx] = manager.Disk(diskName)
			}(i)
		}

		wg.Wait()

		// Verify all disks were created
		for i, disk := range results {
			if disk == nil {
				t.Errorf("disk %d is nil", i)
			}
		}
	})
}

func TestManagerPanicOnInvalidDisk(t *testing.T) {
	cfg := &mockConfig{
		data: map[string]any{
			"filesystem.default": "invalid",
			"filesystem.disks.invalid": map[string]any{
				"driver": "nonexistent",
			},
		},
	}
	manager := NewManager(cfg)

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic when resolving invalid disk")
		}
	}()

	manager.Disk()
}

func TestManagerWithS3Driver(t *testing.T) {
	t.Run("resolve s3 driver with registered extension", func(t *testing.T) {
		cfg := &mockConfig{
			data: map[string]any{
				"filesystem.disks.s3": map[string]any{
					"driver": "s3",
					"bucket": "test-bucket",
					"region": "us-east-1",
				},
			},
		}
		manager := NewManager(cfg)

		// Register S3 driver
		manager.Extend("s3", func(config map[string]any) (contracts.Filesystem, error) {
			return &mockFilesystem{name: "s3"}, nil
		})

		disk, err := manager.resolve("s3")
		if err != nil {
			t.Fatalf("failed to resolve s3 driver: %v", err)
		}
		if disk == nil {
			t.Fatal("expected disk instance, got nil")
		}
	})

	t.Run("resolve s3 driver with built-in", func(t *testing.T) {
		cfg := &mockConfig{
			data: map[string]any{
				"filesystem.disks.s3": map[string]any{
					"driver": "s3",
					"bucket": "test-bucket",
					"region": "us-east-1",
					"key":    "test-key",
					"secret": "test-secret",
				},
			},
		}
		manager := NewManager(cfg)

		// This will use the built-in NewS3 function
		// We can't fully test it without AWS credentials, but we can verify it attempts to create
		_, err := manager.resolve("s3")
		// May error due to missing/invalid credentials, but shouldn't error on "not supported"
		if err != nil && strings.Contains(err.Error(), "not supported") {
			t.Errorf("s3 driver should be supported, got: %v", err)
		}
	})
}

func TestManagerCaching(t *testing.T) {
	manager, _ := setupManager(t)

	t.Run("disk instances are cached", func(t *testing.T) {
		disk1 := manager.Disk("local")
		disk2 := manager.Disk("local")
		disk3 := manager.Disk("local")

		if disk1 != disk2 || disk2 != disk3 {
			t.Error("expected all disk instances to be the same (cached)")
		}

		// Verify only one instance in cache
		if len(manager.disks) != 1 {
			t.Errorf("expected 1 cached disk, got %d", len(manager.disks))
		}
	})

	t.Run("multiple disks are cached separately", func(t *testing.T) {
		disk1 := manager.Disk("local")
		disk2 := manager.Disk("public")
		disk3 := manager.Disk("local")
		disk4 := manager.Disk("public")

		if disk1 != disk3 {
			t.Error("expected local disk instances to be the same")
		}
		if disk2 != disk4 {
			t.Error("expected public disk instances to be the same")
		}
		if disk1 == disk2 {
			t.Error("expected different instances for different disks")
		}

		if len(manager.disks) != 2 {
			t.Errorf("expected 2 cached disks, got %d", len(manager.disks))
		}
	})
}

func TestManagerConfigEdgeCases(t *testing.T) {
	t.Run("empty default driver", func(t *testing.T) {
		cfg := &mockConfig{
			data: map[string]any{
				"filesystem.default": "",
				"filesystem.disks.": map[string]any{
					"driver": "local",
					"root":   "/tmp",
				},
			},
		}
		manager := NewManager(cfg)

		defaultDriver := manager.getDefaultDriver()
		if defaultDriver != "" {
			t.Errorf("expected empty default driver, got '%s'", defaultDriver)
		}
	})

	t.Run("nested config values", func(t *testing.T) {
		cfg := &mockConfig{
			data: map[string]any{
				"filesystem.disks.nested": map[string]any{
					"driver": "local",
					"root":   "/tmp",
					"options": map[string]any{
						"visibility": "public",
						"permissions": map[string]any{
							"file": 0644,
							"dir":  0755,
						},
					},
				},
			},
		}
		manager := NewManager(cfg)

		config := manager.getConfig("nested")
		if config == nil {
			t.Fatal("expected config map, got nil")
		}

		options, ok := config["options"].(map[string]any)
		if !ok {
			t.Fatal("expected nested options map")
		}

		if options["visibility"] != "public" {
			t.Errorf("expected visibility 'public', got '%v'", options["visibility"])
		}
	})
}
