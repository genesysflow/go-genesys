package log_test

import (
	"testing"

	"github.com/genesysflow/go-genesys/facades/log"
	"github.com/genesysflow/go-genesys/testutil"
	"github.com/stretchr/testify/assert"
)

func TestFacadeDelegates(t *testing.T) {
	log.SetInstance(&testutil.MockLogger{})
	t.Cleanup(func() { log.SetInstance(nil) })

	// The mock logger swallows output; this verifies delegation does not
	// panic and WithFields returns a usable logger.
	log.Debug("d")
	log.Info("i", "key", "value")
	log.Warn("w")
	log.Error("e")
	assert.NotNil(t, log.WithFields(map[string]any{"a": 1}))
	assert.NotNil(t, log.GetInstance())
}

func TestFacadePanicsWithoutInstance(t *testing.T) {
	log.SetInstance(nil)
	assert.Panics(t, func() { log.Info("x") })
}
