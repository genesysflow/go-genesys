package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/genesysflow/go-genesys/query"
)

// Password reset errors.
var (
	// ErrResetThrottled means a token was requested again too soon.
	ErrResetThrottled = errors.New("auth: password reset recently requested, try again later")
	// ErrInvalidResetToken means the token does not match.
	ErrInvalidResetToken = errors.New("auth: invalid password reset token")
	// ErrExpiredResetToken means the token matched but is too old.
	ErrExpiredResetToken = errors.New("auth: expired password reset token")
)

// PasswordBroker issues and redeems password reset tokens, Laravel's
// Password broker. Tokens are high-entropy, stored hashed, throttled
// per email, and expire.
//
// The table needs columns: email (unique), token, created_at.
type PasswordBroker struct {
	driver   string
	executor query.Executor
	table    string

	// Expiry is how long a token stays valid (default 60 minutes).
	Expiry time.Duration

	// Throttle is the minimum wait between token requests for the same
	// email (default 60 seconds). Zero disables throttling.
	Throttle time.Duration
}

// NewPasswordBroker creates a broker over the given connection. An
// empty table name defaults to "password_reset_tokens".
func NewPasswordBroker(driver string, executor query.Executor, table string) *PasswordBroker {
	if table == "" {
		table = "password_reset_tokens"
	}
	return &PasswordBroker{
		driver:   driver,
		executor: executor,
		table:    table,
		Expiry:   time.Hour,
		Throttle: time.Minute,
	}
}

func (b *PasswordBroker) query() *query.Builder {
	return query.New(b.driver, b.executor).Table(b.table)
}

func hashResetToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// CreateToken issues a reset token for the email, replacing any
// existing one. The returned plaintext token goes into the reset link;
// only its hash is stored.
func (b *PasswordBroker) CreateToken(email string) (string, error) {
	if b.Throttle > 0 {
		rows, err := b.query().Where("email", email).Get()
		if err != nil {
			return "", err
		}
		if len(rows) > 0 {
			if createdAt, ok := parseBrokerTime(rows[0]["created_at"]); ok {
				if time.Since(createdAt) < b.Throttle {
					return "", ErrResetThrottled
				}
			}
		}
	}

	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := hex.EncodeToString(raw)

	if _, err := b.query().Where("email", email).Delete(); err != nil {
		return "", err
	}
	if err := b.query().Insert(map[string]any{
		"email":      email,
		"token":      hashResetToken(token),
		"created_at": time.Now().UTC(),
	}); err != nil {
		return "", err
	}
	return token, nil
}

// Verify checks a token without consuming it.
func (b *PasswordBroker) Verify(email, token string) error {
	rows, err := b.query().Where("email", email).Get()
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return ErrInvalidResetToken
	}
	stored := fmt.Sprint(rows[0]["token"])
	if subtle.ConstantTimeCompare([]byte(stored), []byte(hashResetToken(token))) != 1 {
		return ErrInvalidResetToken
	}
	if createdAt, ok := parseBrokerTime(rows[0]["created_at"]); ok {
		if time.Since(createdAt) > b.Expiry {
			return ErrExpiredResetToken
		}
	}
	return nil
}

// Consume verifies the token and deletes it, so a reset link can only
// be used once. Run the actual password change in fn; the token is kept
// when fn fails.
func (b *PasswordBroker) Consume(email, token string, fn func() error) error {
	if err := b.Verify(email, token); err != nil {
		return err
	}
	if fn != nil {
		if err := fn(); err != nil {
			return err
		}
	}
	return b.DeleteToken(email)
}

// DeleteToken removes any reset token for the email.
func (b *PasswordBroker) DeleteToken(email string) error {
	_, err := b.query().Where("email", email).Delete()
	return err
}

// parseBrokerTime handles the timestamp shapes drivers return.
func parseBrokerTime(value any) (time.Time, bool) {
	switch v := value.(type) {
	case time.Time:
		return v, true
	case []byte:
		return parseBrokerTime(string(v))
	case string:
		for _, layout := range []string{
			time.RFC3339Nano, time.RFC3339,
			"2006-01-02 15:04:05.999999999-07:00",
			"2006-01-02 15:04:05.999999999",
			"2006-01-02 15:04:05",
		} {
			if parsed, err := time.Parse(layout, v); err == nil {
				return parsed, true
			}
		}
	}
	return time.Time{}, false
}
