package commands_test

import (
	"testing"

	"github.com/genesysflow/go-genesys/cache"
	"github.com/genesysflow/go-genesys/console/commands"
	"github.com/genesysflow/go-genesys/container"
	"github.com/genesysflow/go-genesys/foundation"
	"github.com/genesysflow/go-genesys/providers"
	"github.com/genesysflow/go-genesys/queue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type alertJob struct {
	Message string `json:"message"`
}

func (j *alertJob) JobName() string { return "alert" }
func (j *alertJob) Handle() error   { return nil }

func queueApp(t *testing.T) (*foundation.Application, *queue.MemoryQueue) {
	t.Helper()

	driver := queue.NewMemoryQueue()

	app := foundation.New()
	require.NoError(t, app.Register(&providers.CacheServiceProvider{}))
	require.NoError(t, app.Register(&providers.QueueServiceProvider{}))
	require.NoError(t, app.Boot())

	manager := container.MustResolve[*queue.Manager](app)
	manager.Register("memory", driver)
	manager.SetDefaultConnection("memory")

	return app, driver
}

func TestQueueMonitorCommand(t *testing.T) {
	app, driver := queueApp(t)

	require.NoError(t, driver.PushOn("high", 0, &alertJob{Message: "one"}))
	require.NoError(t, driver.PushOn("high", 0, &alertJob{Message: "two"}))
	require.NoError(t, driver.PushOn("default", 0, &alertJob{Message: "three"}))

	out, err := runCommand(t, app, commands.QueueMonitorCommand(app), "high,default")
	require.NoError(t, err)

	assert.Contains(t, out, "high")
	assert.Contains(t, out, "2")
	assert.Contains(t, out, "default")
	assert.Contains(t, out, "1")
}

// A queue over its threshold is called out, which is the whole point of
// monitoring it.
func TestQueueMonitorReportsBacklog(t *testing.T) {
	app, driver := queueApp(t)

	for i := 0; i < 5; i++ {
		require.NoError(t, driver.PushOn("high", 0, &alertJob{Message: "backlog"}))
	}

	out, err := runCommand(t, app, commands.QueueMonitorCommand(app), "high", "--max", "3")
	require.NoError(t, err)
	assert.Contains(t, out, "above")
}

// A driver that cannot report its size says so rather than reporting an
// empty queue.
func TestQueueMonitorUnsupportedDriver(t *testing.T) {
	app := foundation.New()
	require.NoError(t, app.Register(&providers.QueueServiceProvider{}))
	require.NoError(t, app.Boot())

	manager := container.MustResolve[*queue.Manager](app)
	manager.Register("sync", queue.NewSyncQueue())
	manager.SetDefaultConnection("sync")

	_, err := runCommand(t, app, commands.QueueMonitorCommand(app), "default")
	assert.Error(t, err)
}

func TestQueueRestartCommand(t *testing.T) {
	app, _ := queueApp(t)

	out, err := runCommand(t, app, commands.QueueRestartCommand(app))
	require.NoError(t, err)
	assert.Contains(t, out, "restart")

	store, err := container.MustResolve[*cache.Manager](app).Store()
	require.NoError(t, err)

	_, signalled := queue.RestartSignalledAt(store)
	assert.True(t, signalled, "the signal should be stored where workers look for it")
}

// Without a cache store there is nowhere to leave the signal, so the
// command fails rather than reporting a restart nobody will see.
func TestQueueRestartWithoutCache(t *testing.T) {
	app := foundation.New()
	require.NoError(t, app.Boot())

	_, err := runCommand(t, app, commands.QueueRestartCommand(app))
	assert.Error(t, err)
}
