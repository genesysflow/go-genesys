package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/genesysflow/go-genesys/cache"
	"github.com/genesysflow/go-genesys/console/cli"
	"github.com/genesysflow/go-genesys/container"
	"github.com/genesysflow/go-genesys/contracts"
)

// CacheClearCommand flushes the application cache, Laravel's
// `artisan cache:clear`.
func CacheClearCommand(app contracts.Application) *cli.Command {
	return &cli.Command{
		Name:        "cache:clear",
		Description: "Flush the application cache",
		Options: []cli.Option{
			{Name: "store", Description: "The cache store to flush", Default: "default"},
		},
		Handle: func(c *cli.Context) error {
			store, name, err := resolveCacheStore(c)
			if err != nil {
				return err
			}
			if err := store.Flush(); err != nil {
				return fmt.Errorf("cache:clear: %w", err)
			}

			c.Info(fmt.Sprintf("Cache store [%s] cleared.", name))
			return nil
		},
	}
}

// CacheForgetCommand removes a single key, Laravel's
// `artisan cache:forget`.
func CacheForgetCommand(app contracts.Application) *cli.Command {
	return &cli.Command{
		Name:        "cache:forget",
		Description: "Remove an item from the cache",
		Arguments: []cli.Argument{
			{Name: "key", Description: "The cache key to remove", Required: true},
		},
		Options: []cli.Option{
			{Name: "store", Description: "The cache store to remove from", Default: "default"},
		},
		Handle: func(c *cli.Context) error {
			store, name, err := resolveCacheStore(c)
			if err != nil {
				return err
			}

			key := c.Argument("key")
			if err := store.Forget(key); err != nil {
				return fmt.Errorf("cache:forget: %w", err)
			}

			c.Info(fmt.Sprintf("Key [%s] removed from store [%s].", key, name))
			return nil
		},
	}
}

// resolveCacheStore returns the store named by --store, and its name.
func resolveCacheStore(c *cli.Context) (cache.Store, string, error) {
	manager, err := container.Resolve[*cache.Manager](c.App())
	if err != nil {
		return nil, "", fmt.Errorf("no cache manager available - register the CacheServiceProvider: %w", err)
	}

	name := c.Option("store")
	if name == "" || name == "default" {
		store, err := manager.Store()
		return store, manager.DefaultStore(), err
	}

	store, err := manager.Store(name)
	return store, name, err
}

// StorageLinkCommand links public/storage to storage/app/public so
// uploaded files are servable, Laravel's `artisan storage:link`.
func StorageLinkCommand(app contracts.Application) *cli.Command {
	return &cli.Command{
		Name:        "storage:link",
		Description: "Create the symbolic link from public/storage to storage/app/public",
		Options: []cli.Option{
			{Name: "force", Description: "Replace an existing link"},
		},
		Handle: func(c *cli.Context) error {
			base := c.App().BasePath()
			target := filepath.Join(base, "storage", "app", "public")
			link := filepath.Join(base, "public", "storage")

			// A link to a directory that does not exist is not a working
			// link, and a fresh checkout has no storage/app/public.
			if err := os.MkdirAll(target, 0o750); err != nil {
				return fmt.Errorf("storage:link: creating %s: %w", target, err)
			}
			if err := os.MkdirAll(filepath.Dir(link), 0o750); err != nil {
				return fmt.Errorf("storage:link: creating %s: %w", filepath.Dir(link), err)
			}

			switch existing, err := os.Lstat(link); {
			case err == nil && existing.Mode()&os.ModeSymlink == 0:
				// A real directory here holds someone's uploads. Removing
				// it to make room for a link would delete them.
				return fmt.Errorf("storage:link: %s exists and is not a symlink; move it aside first", link)

			case err == nil:
				current, readErr := os.Readlink(link)
				if readErr == nil && current == target && !c.BoolOption("force") {
					c.Line(fmt.Sprintf("The [public/storage] link already points at [%s].", target))
					return nil
				}
				if !c.BoolOption("force") {
					return fmt.Errorf("storage:link: %s already exists; pass --force to replace it", link)
				}
				if err := os.Remove(link); err != nil {
					return fmt.Errorf("storage:link: replacing %s: %w", link, err)
				}

			case !os.IsNotExist(err):
				return fmt.Errorf("storage:link: %s: %w", link, err)
			}

			if err := os.Symlink(target, link); err != nil {
				return fmt.Errorf("storage:link: %w", err)
			}

			c.Info(fmt.Sprintf("The [public/storage] link has been connected to [%s].", target))
			return nil
		},
	}
}

// secretKeyParts mark a configuration key as holding a credential. They
// are matched against the last segment of the key.
var secretKeyParts = []string{"password", "secret", "token", "key"}

// ConfigShowCommand prints a configuration section or value, Laravel's
// `artisan config:show`.
//
// Laravel's config:cache, route:cache, and view:cache have no analogue
// here: configuration is parsed once at boot, and routes and views are
// Go code compiled into the binary.
func ConfigShowCommand(app contracts.Application) *cli.Command {
	return &cli.Command{
		Name:        "config:show",
		Description: "Display a configuration section or value",
		Arguments: []cli.Argument{
			{Name: "key", Description: "The configuration key, e.g. mail or mail.host", Required: true},
		},
		Options: []cli.Option{
			{Name: "show-secrets", Description: "Print credential values instead of masking them"},
		},
		Handle: func(c *cli.Context) error {
			cfg := c.App().GetConfig()
			if cfg == nil {
				return fmt.Errorf("config:show: no configuration loaded")
			}

			key := c.Argument("key")
			value := cfg.Get(key)
			if value == nil {
				c.Warn(fmt.Sprintf("Configuration key [%s] is not set.", key))
				return nil
			}

			rows := flattenConfig(key, value, c.BoolOption("show-secrets"))
			sort.Slice(rows, func(i, j int) bool { return rows[i][0] < rows[j][0] })

			c.Table([]string{"Key", "Value"}, rows)
			return nil
		},
	}
}

// flattenConfig renders a value as key/value rows, masking credentials
// unless showSecrets is set - config:show is run in screen-shares.
func flattenConfig(prefix string, value any, showSecrets bool) [][]string {
	switch typed := value.(type) {
	case map[string]any:
		rows := make([][]string, 0, len(typed))
		for key, nested := range typed {
			rows = append(rows, flattenConfig(prefix+"."+key, nested, showSecrets)...)
		}
		return rows

	case map[any]any:
		rows := make([][]string, 0, len(typed))
		for key, nested := range typed {
			rows = append(rows, flattenConfig(fmt.Sprintf("%s.%v", prefix, key), nested, showSecrets)...)
		}
		return rows

	default:
		rendered := fmt.Sprint(value)
		if !showSecrets && isSecretKey(prefix) && rendered != "" {
			rendered = strings.Repeat("*", 8)
		}
		return [][]string{{prefix, rendered}}
	}
}

// isSecretKey reports whether a key's last segment names a credential.
func isSecretKey(key string) bool {
	segments := strings.Split(key, ".")
	last := strings.ToLower(segments[len(segments)-1])

	for _, part := range secretKeyParts {
		if strings.Contains(last, part) {
			return true
		}
	}
	return false
}
