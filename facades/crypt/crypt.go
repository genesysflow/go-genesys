// Package crypt provides a static facade for encryption.
package crypt

import (
	"sync"

	basecrypt "github.com/genesysflow/go-genesys/crypt"
)

var (
	instance *basecrypt.Encrypter
	mu       sync.RWMutex
)

// SetInstance sets the encrypter instance.
// This is called during application bootstrap.
func SetInstance(encrypter *basecrypt.Encrypter) {
	mu.Lock()
	defer mu.Unlock()
	instance = encrypter
}

// GetInstance returns the encrypter instance.
func GetInstance() *basecrypt.Encrypter {
	mu.RLock()
	defer mu.RUnlock()
	return instance
}

func encrypter() *basecrypt.Encrypter {
	mu.RLock()
	defer mu.RUnlock()
	if instance == nil {
		panic("crypt: facade not initialised - set APP_KEY and register the EncryptionServiceProvider")
	}
	return instance
}

// Encrypt encrypts plaintext bytes.
func Encrypt(plaintext []byte) (string, error) {
	return encrypter().Encrypt(plaintext)
}

// EncryptString encrypts a string.
func EncryptString(plaintext string) (string, error) {
	return encrypter().EncryptString(plaintext)
}

// Decrypt decrypts a payload.
func Decrypt(payload string) ([]byte, error) {
	return encrypter().Decrypt(payload)
}

// DecryptString decrypts a payload into a string.
func DecryptString(payload string) (string, error) {
	return encrypter().DecryptString(payload)
}
