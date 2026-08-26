package providers

import (
	"fmt"

	"github.com/genesysflow/go-genesys/auth"
	"github.com/genesysflow/go-genesys/container"
	"github.com/genesysflow/go-genesys/contracts"
	"github.com/genesysflow/go-genesys/database"
	"github.com/genesysflow/go-genesys/events"
	"github.com/genesysflow/go-genesys/example/app/jobs"
	"github.com/genesysflow/go-genesys/example/app/models"
	"github.com/genesysflow/go-genesys/example/app/policies"
	"github.com/genesysflow/go-genesys/queue"
	"github.com/genesysflow/go-genesys/view"
)

// BlogServiceProvider wires the blog itself: its authorization policies,
// the polymorphic type registry, the queued jobs and the event
// listeners.
type BlogServiceProvider struct{}

// Register makes the jobs and morph types resolvable. Both are lookups
// by name, so they have to exist before anything reads a payload or a
// morph column.
func (p *BlogServiceProvider) Register(app contracts.Application) error {
	jobs.Register()

	// Go cannot turn a stored type string back into a struct type, so
	// every model a morphTo can point at is registered here.
	database.RegisterMorph[models.Post]()
	database.RegisterMorph[models.Comment]()

	return nil
}

// Boot registers the policies, the listeners and the view helpers the
// blog's pages need - all of which depend on services the framework
// providers build, so none of them can be wired at Register time.
func (p *BlogServiceProvider) Boot(app contracts.Application) error {
	// text/template cannot add two numbers, and the pagination links
	// need the page either side of the current one. Registered here
	// because the view manager is only built in the view provider's
	// Boot, which runs before this one.
	if views, err := container.Resolve[*view.Manager](app); err == nil {
		views.AddFunc("add", func(a, b int) int { return a + b })
		views.AddFunc("sub", func(a, b int) int { return a - b })
	}

	gate, err := container.Resolve[*auth.Gate](app)
	if err != nil {
		return fmt.Errorf("blog: no gate available: %w", err)
	}

	if err := auth.RegisterPolicy[models.Post](gate, &policies.PostPolicy{}); err != nil {
		return err
	}

	// Editors may do anything: the before hook short-circuits every
	// check, so the policies do not each have to know about the role.
	gate.Before(func(user auth.Authenticatable, ability string, args ...any) *bool {
		editor, ok := models.AsUser(user)
		if !ok || !editor.IsEditor() {
			return nil
		}
		allowed := true
		return &allowed
	})

	dispatcher, err := container.Resolve[*events.Dispatcher](app)
	if err != nil {
		return nil
	}

	// When a post goes live, queue the follow-up work rather than doing
	// it in the request that published it.
	dispatcher.Listen("post.published", func(event events.Event) error {
		published, ok := event.(interface{ PostIdentifier() int64 })
		if !ok {
			return nil
		}

		manager, err := container.Resolve[*queue.Manager](app)
		if err != nil {
			return nil
		}
		connection, err := manager.Connection()
		if err != nil {
			return err
		}

		return connection.Push(&jobs.PublishPost{PostID: published.PostIdentifier()})
	})

	return nil
}

// Provides returns the services this provider registers.
func (p *BlogServiceProvider) Provides() []string {
	return []string{"blog"}
}
