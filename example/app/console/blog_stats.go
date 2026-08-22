// Package console holds the example application's own commands.
package console

import (
	"fmt"
	"time"

	"github.com/genesysflow/go-genesys/cache"
	"github.com/genesysflow/go-genesys/console/cli"
	"github.com/genesysflow/go-genesys/container"
	"github.com/genesysflow/go-genesys/contracts"
	"github.com/genesysflow/go-genesys/database"
	"github.com/genesysflow/go-genesys/example/app/models"
	"github.com/genesysflow/go-genesys/support"
)

// statsCacheKey holds the computed figures between runs.
const statsCacheKey = "blog:stats"

// BlogStatsCommand prints what the blog holds. It shows the command
// base: arguments, options, a table, a confirmation and cached work.
func BlogStatsCommand(app contracts.Application) *cli.Command {
	return &cli.Command{
		Name:        "blog:stats",
		Description: "Show how many posts, comments and authors the blog holds",
		Options: []cli.Option{
			{Name: "fresh", Description: "Recompute instead of reading the cache"},
			{Name: "since", Description: "Only count posts published after this date (YYYY-MM-DD)", Default: ""},
		},
		Handle: func(c *cli.Context) error {
			if c.BoolOption("fresh") {
				if store, err := cacheStore(c.App()); err == nil {
					if err := store.Forget(statsCacheKey); err != nil {
						return err
					}
				}
			}

			stats, err := blogStats(c.App(), c.Option("since"))
			if err != nil {
				return err
			}

			c.Table([]string{"Metric", "Value"}, [][]string{
				{"Authors", support.Num.Format(stats.Authors)},
				{"Posts", support.Num.Format(stats.Posts)},
				{"Published", support.Num.Format(stats.Published)},
				{"Comments", support.Num.Format(stats.Comments)},
				{"Published share", support.Num.Percentage(stats.PublishedShare())},
			})

			if stats.Posts == 0 {
				c.Warn("No posts yet. Run `db:seed` to fill the blog.")
			}
			return nil
		},
	}
}

// stats are the figures the command prints.
type stats struct {
	Authors   int64
	Posts     int64
	Published int64
	Comments  int64
}

// PublishedShare is the percentage of posts that are live.
func (s stats) PublishedShare() float64 {
	if s.Posts == 0 {
		return 0
	}
	return float64(s.Published) / float64(s.Posts) * 100
}

// blogStats counts the blog, caching the answer for a minute: the
// command is cheap to run repeatedly and the figures move slowly.
func blogStats(app contracts.Application, since string) (stats, error) {
	compute := func() (stats, error) {
		authors, err := database.Query[models.User]().Count()
		if err != nil {
			return stats{}, err
		}

		posts := database.Query[models.Post]()
		published := database.Query[models.Post]().WhereNotNull("published_at")
		if since != "" {
			if _, err := time.Parse("2006-01-02", since); err != nil {
				return stats{}, fmt.Errorf("blog:stats: --since must be YYYY-MM-DD: %w", err)
			}
			published = published.Where("published_at", ">=", since)
		}

		total, err := posts.Count()
		if err != nil {
			return stats{}, err
		}
		live, err := published.Count()
		if err != nil {
			return stats{}, err
		}
		comments, err := database.Query[models.Comment]().Count()
		if err != nil {
			return stats{}, err
		}

		return stats{Authors: authors, Posts: total, Published: live, Comments: comments}, nil
	}

	// A --since run is not cached: the answer depends on the argument.
	store, err := cacheStore(app)
	if err != nil || since != "" {
		return compute()
	}

	cached, err := cache.Remember(store, statsCacheKey, time.Minute, func() (any, error) {
		return compute()
	})
	if err != nil {
		return stats{}, err
	}

	if figures, ok := cached.(stats); ok {
		return figures, nil
	}
	return compute()
}

// cacheStore resolves the application's default cache store.
func cacheStore(app contracts.Application) (cache.Store, error) {
	manager, err := container.Resolve[*cache.Manager](app)
	if err != nil {
		return nil, err
	}
	return manager.Store()
}
