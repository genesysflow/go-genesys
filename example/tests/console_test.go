package tests

import (
	"bytes"
	"testing"

	"github.com/genesysflow/go-genesys/console/cli"
	appconsole "github.com/genesysflow/go-genesys/example/app/console"
	"github.com/genesysflow/go-genesys/example/app/tasks"
	"github.com/genesysflow/go-genesys/testutil/dbtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// run executes one of the application's commands and returns what it
// printed.
func (h *harness) run(t *testing.T, command *cli.Command, argv ...string) (string, error) {
	t.Helper()

	var out bytes.Buffer
	cobra := command.Cobra(h.app)
	cobra.SetOut(&out)
	cobra.SetErr(&out)
	cobra.SetArgs(argv)

	err := cobra.Execute()
	return out.String(), err
}

// blog:stats counts what the blog holds.
func TestBlogStatsCommand(t *testing.T) {
	h := boot(t)
	author := h.author(t)
	h.published(t, author)
	h.published(t, author)
	h.post(t, author)

	out, err := h.run(t, appconsole.BlogStatsCommand(h.app), "--fresh")
	require.NoError(t, err)

	assert.Contains(t, out, "Authors")
	assert.Contains(t, out, "Published share")
	assert.Contains(t, out, "66.7%")
}

// An empty blog says so rather than printing a table of zeroes and
// leaving the reader to work out what happened.
func TestBlogStatsWarnsWhenTheBlogIsEmpty(t *testing.T) {
	h := boot(t)

	out, err := h.run(t, appconsole.BlogStatsCommand(h.app), "--fresh")
	require.NoError(t, err)
	assert.Contains(t, out, "db:seed")
}

// --since only counts what was published after the date, and refuses a
// date it cannot read rather than counting the wrong thing.
func TestBlogStatsSinceOption(t *testing.T) {
	h := boot(t)
	h.published(t, h.author(t))

	out, err := h.run(t, appconsole.BlogStatsCommand(h.app), "--since", "2999-01-01")
	require.NoError(t, err)
	assert.Contains(t, out, "0.0%")

	_, err = h.run(t, appconsole.BlogStatsCommand(h.app), "--since", "last tuesday")
	assert.Error(t, err)
}

// The prune task soft-deletes drafts nobody has touched in months, and
// leaves everything else alone.
func TestPruneStaleDraftsTask(t *testing.T) {
	h := boot(t)
	author := h.author(t)

	stale := h.post(t, author)
	fresh := h.post(t, author)
	live := h.published(t, author)

	h.age(t, "posts", stale.ID, "-200 days")
	h.age(t, "posts", live.ID, "-200 days")

	require.NoError(t, tasks.PruneStaleDrafts())

	dbtest.AssertSoftDeleted(t, "posts", map[string]any{"id": stale.ID})
	dbtest.AssertDatabaseHas(t, "posts", map[string]any{"id": fresh.ID, "deleted_at": nil})
	dbtest.AssertDatabaseHas(t, "posts", map[string]any{"id": live.ID, "deleted_at": nil})
}

// The digest task hands the work to the queue rather than doing it.
func TestQueueWeeklyDigestTask(t *testing.T) {
	h := boot(t)

	require.NoError(t, tasks.QueueWeeklyDigest(h.app)())

	assert.Equal(t, int64(1), h.queued(t))
}
