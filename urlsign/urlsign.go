// Package urlsign creates and verifies signed URLs (Laravel's signed
// routes): tamper-proof links, optionally with an expiry, authenticated by
// an HMAC derived from the application key.
package urlsign

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Signer signs and verifies URLs.
type Signer struct {
	key []byte
}

// New creates a signer from a raw key (use crypt.ParseKey for APP_KEY).
func New(key []byte) *Signer {
	return &Signer{key: key}
}

// Sign returns the URL with a signature query parameter appended.
func (s *Signer) Sign(rawURL string) (string, error) {
	return s.sign(rawURL, 0)
}

// SignTemporary returns a signed URL that expires after ttl.
func (s *Signer) SignTemporary(rawURL string, ttl time.Duration) (string, error) {
	return s.sign(rawURL, time.Now().Add(ttl).Unix())
}

func (s *Signer) sign(rawURL string, expires int64) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("urlsign: invalid url: %w", err)
	}

	query := parsed.Query()
	if query.Has("signature") {
		return "", fmt.Errorf("urlsign: url already has a signature parameter")
	}
	if expires > 0 {
		query.Set("expires", strconv.FormatInt(expires, 10))
	}

	query.Set("signature", s.signatureFor(parsed.Path, query))
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

// Verify reports whether the URL's signature is valid and unexpired.
func (s *Signer) Verify(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}

	query := parsed.Query()
	signature := query.Get("signature")
	if signature == "" {
		return false
	}
	query.Del("signature")

	expected := s.signatureFor(parsed.Path, query)
	if !hmac.Equal([]byte(signature), []byte(expected)) {
		return false
	}

	if expiresRaw := query.Get("expires"); expiresRaw != "" {
		expires, err := strconv.ParseInt(expiresRaw, 10, 64)
		if err != nil || time.Now().Unix() > expires {
			return false
		}
	}
	return true
}

// signatureFor computes the HMAC over the path and canonically-ordered
// query string (signature parameter excluded).
func (s *Signer) signatureFor(path string, query url.Values) string {
	keys := make([]string, 0, len(query))
	for key := range query {
		if key != "signature" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)

	var canonical strings.Builder
	canonical.WriteString(path)
	for _, key := range keys {
		values := append([]string(nil), query[key]...)
		sort.Strings(values)
		for _, value := range values {
			canonical.WriteString("&" + key + "=" + value)
		}
	}

	mac := hmac.New(sha256.New, s.key)
	mac.Write([]byte(canonical.String()))
	return hex.EncodeToString(mac.Sum(nil))
}
