package auth_test

import (
	"os"
	"testing"

	"github.com/genesysflow/go-genesys/hash"
	"golang.org/x/crypto/bcrypt"
)

// TestMain drops the bcrypt cost to its minimum: at the production cost
// each hash takes seconds under the race detector, and nothing here
// depends on the work factor.
func TestMain(m *testing.M) {
	hash.SetDefaultCost(bcrypt.MinCost)
	os.Exit(m.Run())
}
