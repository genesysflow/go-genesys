// Package main demonstrates Go-Genesys framework usage.
package main

import (
	"fmt"
	"os"

	"github.com/genesysflow/go-genesys/console"
	"github.com/genesysflow/go-genesys/example/bootstrap"
)

func main() {
	app := bootstrap.App()

	if err := app.Boot(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	kernel, err := app.Make("console.kernel")
	if err != nil {
		fmt.Fprintln(os.Stderr, "Failed to resolve console kernel:", err)
		os.Exit(1)
	}

	if err := kernel.(*console.Kernel).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
