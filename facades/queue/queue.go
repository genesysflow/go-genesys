// Package queue provides a static facade for dispatching jobs.
package queue

import (
	"sync"
	"time"

	basequeue "github.com/genesysflow/go-genesys/queue"
)

var (
	instance *basequeue.Manager
	mu       sync.RWMutex
)

// SetInstance sets the queue manager instance.
// This is called during application bootstrap.
func SetInstance(manager *basequeue.Manager) {
	mu.Lock()
	defer mu.Unlock()
	instance = manager
}

// GetInstance returns the queue manager instance.
func GetInstance() *basequeue.Manager {
	mu.RLock()
	defer mu.RUnlock()
	return instance
}

func connection(name ...string) basequeue.Queue {
	mu.RLock()
	manager := instance
	mu.RUnlock()
	if manager == nil {
		panic("queue: facade not initialised - register the QueueServiceProvider")
	}
	conn, err := manager.Connection(name...)
	if err != nil {
		panic(err)
	}
	return conn
}

// Connection returns a queue connection by name (default when omitted).
func Connection(name ...string) basequeue.Queue {
	return connection(name...)
}

// Dispatch pushes a job onto the default queue connection.
func Dispatch(job basequeue.Job) error {
	return connection().Push(job)
}

// DispatchLater pushes a job to run after the given delay.
func DispatchLater(delay time.Duration, job basequeue.Job) error {
	return connection().Later(delay, job)
}

// DispatchOn dispatches a job onto a named queue of the default
// connection - Laravel's onQueue:
//
//	queue.DispatchOn("high", &SendAlert{})
func DispatchOn(queueName string, job basequeue.Job) error {
	return basequeue.PushOn(connection(), queueName, job)
}

// DispatchLaterOn dispatches a delayed job onto a named queue of the
// default connection.
func DispatchLaterOn(queueName string, delay time.Duration, job basequeue.Job) error {
	return basequeue.LaterOn(connection(), queueName, delay, job)
}
