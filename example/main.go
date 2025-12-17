// Package main demonstrates Go-Genesys framework usage.
package main

import (
	"fmt"
	"os"

	"github.com/genesysflow/go-genesys/console"
	"github.com/genesysflow/go-genesys/container"
	"github.com/genesysflow/go-genesys/example/bootstrap"
)

func main() {
	app := bootstrap.App()

	if err := app.Boot(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	// Example of using container.MustResolve to avoid manual type assertion
	kernel := container.MustResolve[*console.Kernel](app, "console.kernel")

	if err := kernel.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
