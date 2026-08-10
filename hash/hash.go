// Package hash provides password hashing and verification.
//
// It exists so that applications do not have to assemble this themselves. A
// password must never be stored as a plain digest — SHA-256, MD5 and friends
// are designed to be fast, which is precisely the property an offline cracker
// wants. bcrypt is deliberately slow and salts every hash, so identical
// passwords produce different stored values and a stolen table cannot be
// attacked with precomputed rainbow tables.
package hash

import (
	"errors"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// ErrMismatch is returned when a password does not match the hash.
var ErrMismatch = errors.New("hash: password does not match")

// DefaultCost is the bcrypt work factor used when none is configured.
//
// 12 is a deliberate step above bcrypt.DefaultCost (10): each increment
// doubles the work an attacker must do per guess, and 12 remains a few tens of
// milliseconds on current server hardware. Raise it as hardware improves.
const DefaultCost = 12

// maxPasswordLength is bcrypt's hard input limit.
//
// bcrypt silently truncates anything past 72 bytes, which would make
// "<72 correct bytes><anything>" verify successfully. Rejecting over-long
// input is safer than accepting a password whose tail is ignored.
const maxPasswordLength = 72

// Hasher hashes and verifies passwords.
type Hasher struct {
	cost int
}

// New creates a Hasher. Cost defaults to DefaultCost when not supplied or out
// of bcrypt's supported range.
func New(cost ...int) *Hasher {
	c := DefaultCost
	if len(cost) > 0 && cost[0] >= bcrypt.MinCost && cost[0] <= bcrypt.MaxCost {
		c = cost[0]
	}
	return &Hasher{cost: c}
}

// Make hashes a plaintext password.
func (h *Hasher) Make(password string) (string, error) {
	if len(password) > maxPasswordLength {
		return "", fmt.Errorf("hash: password exceeds %d bytes", maxPasswordLength)
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(password), h.cost)
	if err != nil {
		return "", fmt.Errorf("hash: failed to hash password: %w", err)
	}

	return string(hashed), nil
}

// Check verifies a plaintext password against a hash.
//
// It returns ErrMismatch for a wrong password and a distinct error if the
// stored hash is unreadable, so that a corrupted record is not silently
// reported as a failed login. bcrypt's comparison is constant-time with
// respect to the hash contents.
func (h *Hasher) Check(password, hashed string) error {
	err := bcrypt.CompareHashAndPassword([]byte(hashed), []byte(password))
	switch {
	case err == nil:
		return nil
	case errors.Is(err, bcrypt.ErrMismatchedHashAndPassword):
		return ErrMismatch
	default:
		return fmt.Errorf("hash: invalid hash: %w", err)
	}
}

// NeedsRehash reports whether a stored hash was produced with a weaker cost
// than the hasher currently uses.
//
// Call it on successful login — the one moment the plaintext is available —
// and re-hash, so existing accounts migrate to a stronger factor over time
// instead of being frozen at whatever cost was current when they signed up.
func (h *Hasher) NeedsRehash(hashed string) bool {
	cost, err := bcrypt.Cost([]byte(hashed))
	if err != nil {
		// Unreadable hash: treat as needing replacement.
		return true
	}
	return cost < h.cost
}

// Cost returns the configured work factor.
func (h *Hasher) Cost() int {
	return h.cost
}

// defaultHasher backs the package-level helpers.
var defaultHasher = New()

// Make hashes a plaintext password using the default hasher.
func Make(password string) (string, error) {
	return defaultHasher.Make(password)
}

// Check verifies a plaintext password against a hash using the default hasher.
func Check(password, hashed string) error {
	return defaultHasher.Check(password, hashed)
}

// NeedsRehash reports whether a hash should be regenerated at the default cost.
func NeedsRehash(hashed string) bool {
	return defaultHasher.NeedsRehash(hashed)
}
