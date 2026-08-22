package providers

import (
	"path/filepath"

	"github.com/genesysflow/go-genesys/contracts"
)

// basePathFor resolves a configured path against the application's base
// path.
//
// Paths in configuration are written relative to the application
// ("resources/views"), not to the process's working directory: a test
// binary runs in its own package directory, and a service runs wherever
// it was launched from. An absolute path is the operator's explicit
// choice and is left alone.
func basePathFor(app contracts.Application, path string) string {
	if path == "" || filepath.IsAbs(path) {
		return path
	}

	base := app.BasePath()
	if base == "" {
		return path
	}

	return filepath.Join(base, filepath.FromSlash(path))
}
