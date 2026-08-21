package container_test

import (
	"testing"

	"github.com/genesysflow/go-genesys/container"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContextualBindings(t *testing.T) {
	c := container.New()
	require.NoError(t, c.Instance("disk", "local-disk"))

	c.When("reports").Needs("disk").GiveValue("s3-disk")

	// The consumer with a contextual binding gets its override...
	forReports, err := c.MakeFor("reports", "disk")
	require.NoError(t, err)
	assert.Equal(t, "s3-disk", forReports)

	// ...everyone else gets the regular binding.
	forOthers, err := c.MakeFor("exports", "disk")
	require.NoError(t, err)
	assert.Equal(t, "local-disk", forOthers)

	direct, err := c.Make("disk")
	require.NoError(t, err)
	assert.Equal(t, "local-disk", direct)
}
