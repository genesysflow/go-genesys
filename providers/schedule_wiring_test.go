package providers_test

import (
	"testing"
	"time"

	"github.com/genesysflow/go-genesys/container"
	"github.com/genesysflow/go-genesys/foundation"
	"github.com/genesysflow/go-genesys/providers"
	"github.com/genesysflow/go-genesys/schedule"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Environments() and OnOneServer() only work if the provider tells the
// scheduler which environment it is in and gives it a cache store.
func TestScheduleProviderWiresEnvironmentAndCache(t *testing.T) {
	t.Setenv("APP_ENV", "production")

	app := foundation.New()
	require.NoError(t, app.Register(&providers.CacheServiceProvider{}))

	var productionOnly, stagingOnly *schedule.Event
	require.NoError(t, app.Register(&providers.ScheduleServiceProvider{
		Define: func(s *schedule.Schedule) {
			productionOnly = s.Call(func() error { return nil }).EveryMinute().Environments("production")
			stagingOnly = s.Call(func() error { return nil }).EveryMinute().Environments("staging")
		},
	}))
	require.NoError(t, app.Boot())

	assert.True(t, productionOnly.IsDue(time.Now()))
	assert.False(t, stagingOnly.IsDue(time.Now()))

	scheduler := container.MustResolve[*schedule.Schedule](app)

	runs := 0
	event := scheduler.Call(func() error { runs++; return nil }).EveryMinute().Description("single").OnOneServer()
	require.NoError(t, event.Run())

	twin := scheduler.Call(func() error { runs++; return nil }).EveryMinute().Description("single").OnOneServer()
	require.NoError(t, twin.Run())

	assert.Equal(t, 1, runs, "the provider should give the scheduler a cache store to coordinate through")
}
