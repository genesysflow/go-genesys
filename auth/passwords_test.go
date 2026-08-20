package auth_test

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/genesysflow/go-genesys/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

type sqliteExec struct{ db *sql.DB }

func (e *sqliteExec) Query(q string, b ...any) (*sql.Rows, error) { return e.db.Query(q, b...) }
func (e *sqliteExec) QueryRow(q string, b ...any) *sql.Row        { return e.db.QueryRow(q, b...) }
func (e *sqliteExec) Exec(q string, b ...any) (sql.Result, error) { return e.db.Exec(q, b...) }

func newBroker(t *testing.T) *auth.PasswordBroker {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	_, err = db.Exec(`CREATE TABLE password_reset_tokens (
		email TEXT PRIMARY KEY, token TEXT NOT NULL, created_at TIMESTAMP NOT NULL)`)
	require.NoError(t, err)
	return auth.NewPasswordBroker("sqlite", &sqliteExec{db: db}, "")
}

func TestPasswordResetHappyPath(t *testing.T) {
	broker := newBroker(t)

	token, err := broker.CreateToken("ada@example.com")
	require.NoError(t, err)
	assert.Len(t, token, 64, "high-entropy token")

	require.NoError(t, broker.Verify("ada@example.com", token))

	reset := false
	require.NoError(t, broker.Consume("ada@example.com", token, func() error {
		reset = true
		return nil
	}))
	assert.True(t, reset)

	// Consumed tokens stop working: single-use links.
	assert.ErrorIs(t, broker.Verify("ada@example.com", token), auth.ErrInvalidResetToken)
}

func TestPasswordResetRejectsBadTokens(t *testing.T) {
	broker := newBroker(t)

	token, err := broker.CreateToken("ada@example.com")
	require.NoError(t, err)

	assert.ErrorIs(t, broker.Verify("ada@example.com", "wrong-token"), auth.ErrInvalidResetToken)
	assert.ErrorIs(t, broker.Verify("other@example.com", token), auth.ErrInvalidResetToken)

	// A failing reset callback keeps the token usable.
	err = broker.Consume("ada@example.com", token, func() error {
		return errors.New("hash failed")
	})
	assert.ErrorContains(t, err, "hash failed")
	assert.NoError(t, broker.Verify("ada@example.com", token))
}

func TestPasswordResetThrottleAndExpiry(t *testing.T) {
	broker := newBroker(t)

	_, err := broker.CreateToken("ada@example.com")
	require.NoError(t, err)

	// Second request inside the throttle window is refused.
	_, err = broker.CreateToken("ada@example.com")
	assert.ErrorIs(t, err, auth.ErrResetThrottled)

	// Outside the window a new token replaces the old one.
	broker.Throttle = time.Millisecond
	time.Sleep(5 * time.Millisecond)
	first, err := broker.CreateToken("ada@example.com")
	require.NoError(t, err)
	time.Sleep(5 * time.Millisecond)
	second, err := broker.CreateToken("ada@example.com")
	require.NoError(t, err)
	assert.NotEqual(t, first, second)
	assert.ErrorIs(t, broker.Verify("ada@example.com", first), auth.ErrInvalidResetToken)

	// Expired tokens are rejected even when they match.
	broker.Expiry = time.Nanosecond
	time.Sleep(time.Millisecond)
	assert.ErrorIs(t, broker.Verify("ada@example.com", second), auth.ErrExpiredResetToken)
}

func TestEmailVerification(t *testing.T) {
	verifier := auth.NewEmailVerifier([]byte("test-app-key-32-bytes-long......"))

	link, err := verifier.VerificationURL("https://app.test/email/verify", 42, "ada@example.com")
	require.NoError(t, err)

	// The link parses and identifies the user.
	id, err := verifier.Parse(link)
	require.NoError(t, err)
	assert.Equal(t, "42", id)

	// The email hash binds the link to the address it was minted for.
	require.NoError(t, verifier.Confirm(link, "ada@example.com"))
	assert.ErrorIs(t, verifier.Confirm(link, "other@example.com"), auth.ErrVerificationEmailMismatch)

	// Tampering breaks the signature.
	tampered := link[:len(link)-4] + "beef"
	_, err = verifier.Parse(tampered)
	assert.ErrorIs(t, err, auth.ErrInvalidVerificationLink)

	// Swapping the id also breaks the signature.
	swapped := replaceOnce(link, "id=42", "id=1")
	_, err = verifier.Parse(swapped)
	assert.ErrorIs(t, err, auth.ErrInvalidVerificationLink)

	// Expired links stop verifying.
	verifier.Expiry = -time.Minute
	expired, err := verifier.VerificationURL("https://app.test/email/verify", 42, "ada@example.com")
	require.NoError(t, err)
	_, err = verifier.Parse(expired)
	assert.ErrorIs(t, err, auth.ErrInvalidVerificationLink)
}

func replaceOnce(s, old, new string) string {
	for i := 0; i+len(old) <= len(s); i++ {
		if s[i:i+len(old)] == old {
			return s[:i] + new + s[i+len(old):]
		}
	}
	return s
}
