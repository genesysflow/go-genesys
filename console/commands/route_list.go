package commands

import (
	"fmt"
	"sort"

	"github.com/genesysflow/go-genesys/container"
	"github.com/genesysflow/go-genesys/contracts"
	"github.com/genesysflow/go-genesys/http"
	"github.com/genesysflow/go-genesys/providers"
	"github.com/spf13/cobra"
)

// RouteListCommand creates the route:list command.
func RouteListCommand(app contracts.Application) *cobra.Command {
	return &cobra.Command{
		Use:   "route:list",
		Short: "List all registered routes",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Register routes the same way `serve` does.
			var routesCallback func(*http.Router)
			if routes, err := container.Resolve[func(*http.Router)](app); err == nil {
				routesCallback = routes
			}
			if routesCallback == nil {
				fmt.Println("No routes registered (bind a func(*http.Router) in the container).")
				return nil
			}

			routeProvider := &providers.RouteServiceProvider{Routes: routesCallback}
			if err := app.Register(routeProvider); err != nil {
				return err
			}
			if err := app.Boot(); err != nil {
				return fmt.Errorf("failed to boot application: %w", err)
			}

			routes := routeProvider.Kernel().Router().Routes()
			if len(routes) == 0 {
				fmt.Println("No routes registered.")
				return nil
			}

			sort.Slice(routes, func(i, j int) bool {
				if routes[i].GetPath() == routes[j].GetPath() {
					return routes[i].GetMethod() < routes[j].GetMethod()
				}
				return routes[i].GetPath() < routes[j].GetPath()
			})

			fmt.Printf("%-8s %-40s %s\n", "METHOD", "PATH", "NAME")
			for _, route := range routes {
				fmt.Printf("%-8s %-40s %s\n", route.GetMethod(), route.GetPath(), route.GetName())
			}
			fmt.Printf("\n%d route(s)\n", len(routes))
			return nil
		},
	}
}
