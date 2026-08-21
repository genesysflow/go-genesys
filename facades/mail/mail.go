// Package mail provides a static facade for sending mail.
package mail

import (
	"sync"

	basemail "github.com/genesysflow/go-genesys/mail"
)

var (
	instance basemail.Mailer
	mu       sync.RWMutex
)

// SetInstance sets the mailer instance.
// This is called during application bootstrap.
func SetInstance(mailer basemail.Mailer) {
	mu.Lock()
	defer mu.Unlock()
	instance = mailer
}

// GetInstance returns the mailer instance.
func GetInstance() basemail.Mailer {
	mu.RLock()
	defer mu.RUnlock()
	return instance
}

func mailer() basemail.Mailer {
	mu.RLock()
	defer mu.RUnlock()
	if instance == nil {
		panic("mail: facade not initialised - register the MailServiceProvider")
	}
	return instance
}

// Send delivers a message through the configured mailer.
func Send(message *basemail.Message) error {
	return mailer().Send(message)
}

// Message creates a new message builder.
func Message() *basemail.Message {
	return basemail.NewMessage()
}
