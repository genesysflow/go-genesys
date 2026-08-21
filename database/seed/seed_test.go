package seed_test

import (
	"errors"
	"testing"

	"github.com/genesysflow/go-genesys/database/seed"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunnerRunsInOrder(t *testing.T) {
	runner := seed.NewRunner()
	var order []string
	runner.AddFunc("users", func() error { order = append(order, "users"); return nil })
	runner.AddFunc("posts", func() error { order = append(order, "posts"); return nil })

	require.NoError(t, runner.Run())
	assert.Equal(t, []string{"users", "posts"}, order)
	assert.Equal(t, []string{"users", "posts"}, runner.Names())
}

func TestRunnerSelectedSeeders(t *testing.T) {
	runner := seed.NewRunner()
	var order []string
	runner.AddFunc("a", func() error { order = append(order, "a"); return nil })
	runner.AddFunc("b", func() error { order = append(order, "b"); return nil })

	require.NoError(t, runner.Run("b"))
	assert.Equal(t, []string{"b"}, order)

	assert.ErrorContains(t, runner.Run("missing"), "not registered")
}

func TestRunnerStopsOnError(t *testing.T) {
	runner := seed.NewRunner()
	var order []string
	runner.AddFunc("first", func() error { return errors.New("boom") })
	runner.AddFunc("second", func() error { order = append(order, "second"); return nil })

	err := runner.Run()
	assert.ErrorContains(t, err, "seeder [first] failed")
	assert.Empty(t, order)
}
