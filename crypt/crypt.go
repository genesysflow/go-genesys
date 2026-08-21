// Package crypt provides authenticated symmetric encryption (AES-256-GCM)
// keyed by the application key, mirroring Laravel's Crypt facade.
package crypt

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

// ErrInvalidPayload is returned when a payload cannot be decrypted.
var ErrInvalidPayload = errors.New("crypt: invalid or tampered payload")

// Encrypter encrypts and decrypts values with AES-GCM.
type Encrypter struct {
	aead cipher.AEAD
}

// New creates an encrypter from a raw key (16, 24, or 32 bytes for
// AES-128/192/256).
func New(key []byte) (*Encrypter, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("crypt: invalid key: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Encrypter{aead: aead}, nil
}

// NewFromString creates an encrypter from an application key string,
// accepting Laravel's "base64:" prefix or a raw string key.
func NewFromString(appKey string) (*Encrypter, error) {
	key, err := ParseKey(appKey)
	if err != nil {
		return nil, err
	}
	return New(key)
}

// ParseKey decodes an application key string into raw bytes.
func ParseKey(appKey string) ([]byte, error) {
	if appKey == "" {
		return nil, errors.New("crypt: application key is empty - run `genesys key:generate`")
	}
	if strings.HasPrefix(appKey, "base64:") {
		key, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(appKey, "base64:"))
		if err != nil {
			return nil, fmt.Errorf("crypt: invalid base64 application key: %w", err)
		}
		return key, nil
	}
	return []byte(appKey), nil
}

// GenerateKey returns a new random 32-byte key in "base64:" format,
// suitable for APP_KEY.
func GenerateKey() string {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		panic("crypt: cannot read random bytes: " + err.Error())
	}
	return "base64:" + base64.StdEncoding.EncodeToString(key)
}

// Encrypt encrypts plaintext and returns a base64 payload of nonce||ciphertext.
func (e *Encrypter) Encrypt(plaintext []byte) (string, error) {
	nonce := make([]byte, e.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := e.aead.Seal(nonce, nonce, plaintext, nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// EncryptString encrypts a string value.
func (e *Encrypter) EncryptString(plaintext string) (string, error) {
	return e.Encrypt([]byte(plaintext))
}

// Decrypt decrypts a payload produced by Encrypt. Tampered or truncated
// payloads return ErrInvalidPayload.
func (e *Encrypter) Decrypt(payload string) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return nil, ErrInvalidPayload
	}
	nonceSize := e.aead.NonceSize()
	if len(raw) < nonceSize {
		return nil, ErrInvalidPayload
	}
	plaintext, err := e.aead.Open(nil, raw[:nonceSize], raw[nonceSize:], nil)
	if err != nil {
		return nil, ErrInvalidPayload
	}
	return plaintext, nil
}

// DecryptString decrypts a payload into a string.
func (e *Encrypter) DecryptString(payload string) (string, error) {
	plaintext, err := e.Decrypt(payload)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}
