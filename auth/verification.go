package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/genesysflow/go-genesys/urlsign"
)

// Email verification errors.
var (
	// ErrInvalidVerificationLink means the signature check failed or the
	// link expired.
	ErrInvalidVerificationLink = errors.New("auth: invalid or expired verification link")
	// ErrVerificationEmailMismatch means the link was minted for a
	// different email address.
	ErrVerificationEmailMismatch = errors.New("auth: verification link does not match this email")
)

// EmailVerifier mints and checks signed email verification links,
// Laravel's MustVerifyEmail flow. Links carry the user id and a hash of
// the email inside a temporary signed URL, so they expire, cannot be
// forged, and stop working if the user changes their address.
type EmailVerifier struct {
	signer *urlsign.Signer

	// Expiry is how long links stay valid (default 60 minutes).
	Expiry time.Duration
}

// NewEmailVerifier creates a verifier keyed with the app key.
func NewEmailVerifier(key []byte) *EmailVerifier {
	return &EmailVerifier{signer: urlsign.New(key), Expiry: time.Hour}
}

func emailHash(email string) string {
	sum := sha256.Sum256([]byte(email))
	return hex.EncodeToString(sum[:])
}

// VerificationURL builds a temporary signed link for the user:
//
//	link, _ := verifier.VerificationURL("https://app.test/email/verify", user.ID, user.Email)
func (v *EmailVerifier) VerificationURL(baseURL string, userID any, email string) (string, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	values := parsed.Query()
	values.Set("id", fmt.Sprint(userID))
	values.Set("hash", emailHash(email))
	parsed.RawQuery = values.Encode()
	return v.signer.SignTemporary(parsed.String(), v.Expiry)
}

// Parse validates a verification link's signature and expiry and
// returns the user id it was minted for. Look the user up by that id,
// then confirm the link matches their address with Confirm.
func (v *EmailVerifier) Parse(rawURL string) (userID string, err error) {
	if !v.signer.Verify(rawURL) {
		return "", ErrInvalidVerificationLink
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", ErrInvalidVerificationLink
	}
	id := parsed.Query().Get("id")
	if id == "" {
		return "", ErrInvalidVerificationLink
	}
	return id, nil
}

// Confirm checks that the link was minted for the given email.
func (v *EmailVerifier) Confirm(rawURL, email string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ErrInvalidVerificationLink
	}
	if parsed.Query().Get("hash") != emailHash(email) {
		return ErrVerificationEmailMismatch
	}
	return nil
}
