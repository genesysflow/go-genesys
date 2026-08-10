package hash

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

// cheap is a minimum-cost hasher. bcrypt at the production cost takes
// hundreds of milliseconds per call by design, which is the point in
// production and pure overhead in tests that only exercise behaviour.
// TestDefaultCostExceedsBcryptDefault covers the real default separately.
var cheap = New(bcrypt.MinCost)

func TestMakeAndCheck(t *testing.T) {
	hashed, err := cheap.Make("correct-horse-battery-staple")
	require.NoError(t, err)

	assert.NoError(t, cheap.Check("correct-horse-battery-staple", hashed))
	assert.ErrorIs(t, cheap.Check("wrong-password", hashed), ErrMismatch)
}

// TestHashIsSalted: identical passwords must produce different stored values,
// so that a stolen table cannot be attacked with precomputed tables and equal
// passwords are not visibly equal.
func TestHashIsSalted(t *testing.T) {
	first, err := cheap.Make("same-password")
	require.NoError(t, err)
	second, err := cheap.Make("same-password")
	require.NoError(t, err)

	assert.NotEqual(t, first, second, "each hash must use a fresh salt")
	assert.NoError(t, cheap.Check("same-password", first))
	assert.NoError(t, cheap.Check("same-password", second))
}

// TestHashDoesNotContainPlaintext guards against an implementation that
// stores or encodes the password itself.
func TestHashDoesNotContainPlaintext(t *testing.T) {
	hashed, err := cheap.Make("unmistakable-plaintext")
	require.NoError(t, err)
	assert.NotContains(t, hashed, "unmistakable-plaintext")
	assert.True(t, strings.HasPrefix(hashed, "$2"), "must be a bcrypt hash, got %q", hashed)
}

// TestDefaultCostExceedsBcryptDefault: each increment doubles an attacker's
// work per guess, so the framework default should sit above the library's.
func TestDefaultCostExceedsBcryptDefault(t *testing.T) {
	assert.Greater(t, DefaultCost, bcrypt.DefaultCost)

	hashed, err := Make("x")
	require.NoError(t, err)

	cost, err := bcrypt.Cost([]byte(hashed))
	require.NoError(t, err)
	assert.Equal(t, DefaultCost, cost)
}

// TestOverLongPasswordRejected: bcrypt silently truncates past 72 bytes, which
// would make "<72 correct bytes><anything>" verify successfully. Rejecting is
// safer than accepting a password whose tail is ignored.
func TestOverLongPasswordRejected(t *testing.T) {
	_, err := cheap.Make(strings.Repeat("a", 73))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "72")
}

func TestPasswordAtLimitAccepted(t *testing.T) {
	password := strings.Repeat("a", 72)
	hashed, err := cheap.Make(password)
	require.NoError(t, err)
	assert.NoError(t, cheap.Check(password, hashed))
}

// TestCheckDistinguishesCorruptHash: a malformed stored hash is an integrity
// problem, not a failed login, and must not be reported as a mismatch.
func TestCheckDistinguishesCorruptHash(t *testing.T) {
	err := cheap.Check("any-password", "not-a-bcrypt-hash")
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrMismatch)
}

// TestNeedsRehash drives the upgrade-on-login path.
func TestNeedsRehash(t *testing.T) {
	weak, err := bcrypt.GenerateFromPassword([]byte("pw"), bcrypt.MinCost)
	require.NoError(t, err)
	assert.True(t, New(bcrypt.MinCost+2).NeedsRehash(string(weak)), "a weaker cost must be flagged")

	current, err := cheap.Make("pw")
	require.NoError(t, err)
	assert.False(t, cheap.NeedsRehash(current), "a current-cost hash must not be flagged")

	assert.True(t, cheap.NeedsRehash("garbage"), "an unreadable hash must be flagged")
}

// TestCustomCostClampedToValidRange checks out-of-range costs fall back to the
// default rather than being passed to bcrypt.
func TestCustomCostClampedToValidRange(t *testing.T) {
	assert.Equal(t, DefaultCost, New(bcrypt.MaxCost+1).Cost())
	assert.Equal(t, DefaultCost, New(bcrypt.MinCost-1).Cost())
	assert.Equal(t, bcrypt.MinCost, New(bcrypt.MinCost).Cost())
}
