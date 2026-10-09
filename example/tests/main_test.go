package tests

import (
	"os"
	"testing"

	"github.com/genesysflow/go-genesys/hash"
	"golang.org/x/crypto/bcrypt"
)

// TestMain drops the bcrypt cost to its minimum. Every factory user, login
// and registration hashes a password, and at the production cost each hash
// takes seconds under the race detector - enough to push the suite past
// go test's default timeout. What the tests check does not depend on the
// work factor.
func TestMain(m *testing.M) {
	hash.SetDefaultCost(bcrypt.MinCost)
	os.Exit(m.Run())
}
