// Package broadcast pushes server events to connected WebSocket
// clients over named channels - Laravel's broadcasting. The in-memory
// hub serves a single process; every instance of a multi-node
// deployment gets its own hub.
//
//	hub := broadcast.New()
//	kernel.Fiber().Get("/ws", hub.Handler())
//
//	hub.Broadcast("orders", "order.shipped", map[string]any{"id": 42})
//
// Clients speak a small JSON protocol:
//
//	-> {"action": "subscribe", "channel": "orders"}
//	<- {"channel": "orders", "event": "genesys:subscribed"}
//	<- {"channel": "orders", "event": "order.shipped", "payload": {"id": 42}}
//
// Channels prefixed "private-" require the hub's authorizer to accept
// the token the client sends with its subscribe frame.
package broadcast

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/websocket/v2"
)

// Message is one frame delivered to subscribers.
type Message struct {
	Channel string `json:"channel"`
	Event   string `json:"event"`
	Payload any    `json:"payload,omitempty"`
}

// inbound is a client -> server frame.
type inbound struct {
	Action  string `json:"action"`
	Channel string `json:"channel"`
	Auth    string `json:"auth,omitempty"`
}

// Authorizer decides whether a token may join a private channel.
type Authorizer func(channel, token string) bool

// Hub tracks channels and their subscribers.
type Hub struct {
	mu        sync.RWMutex
	channels  map[string]map[*client]bool
	authorize Authorizer
}

// client is one connected WebSocket peer.
type client struct {
	sendMu   sync.Mutex
	closed   bool
	send     chan []byte
	channels map[string]bool
}

// trySend queues a frame unless the client is closed or its buffer is
// full; it reports whether the frame was queued.
func (c *client) trySend(frame []byte) bool {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	if c.closed {
		return false
	}
	select {
	case c.send <- frame:
		return true
	default:
		return false
	}
}

// close shuts the client's outbox exactly once.
func (c *client) close() {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	if !c.closed {
		c.closed = true
		close(c.send)
	}
}

// New creates a hub.
func New() *Hub {
	return &Hub{channels: make(map[string]map[*client]bool)}
}

// Authorize installs the private-channel authorizer. Without one, all
// "private-" channels reject subscriptions.
func (h *Hub) Authorize(fn Authorizer) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.authorize = fn
}

// IsPrivate reports whether a channel requires authorization.
func IsPrivate(channel string) bool {
	return strings.HasPrefix(channel, "private-")
}

// Broadcast sends an event to every subscriber of a channel. Slow
// clients whose buffers are full are disconnected rather than blocking
// the broadcast.
func (h *Hub) Broadcast(channel, event string, payload any) error {
	frame, err := json.Marshal(Message{Channel: channel, Event: event, Payload: payload})
	if err != nil {
		return fmt.Errorf("broadcast: cannot encode payload: %w", err)
	}

	h.mu.RLock()
	subscribers := make([]*client, 0, len(h.channels[channel]))
	for c := range h.channels[channel] {
		subscribers = append(subscribers, c)
	}
	h.mu.RUnlock()

	for _, c := range subscribers {
		if !c.trySend(frame) {
			h.drop(c) // slow or closed consumer
		}
	}
	return nil
}

// Subscribers returns how many clients are on a channel.
func (h *Hub) Subscribers(channel string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.channels[channel])
}

func (h *Hub) subscribe(c *client, channel, token string) error {
	if IsPrivate(channel) {
		h.mu.RLock()
		authorize := h.authorize
		h.mu.RUnlock()
		if authorize == nil || !authorize(channel, token) {
			return fmt.Errorf("unauthorized for channel %s", channel)
		}
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.channels[channel] == nil {
		h.channels[channel] = make(map[*client]bool)
	}
	h.channels[channel][c] = true
	c.channels[channel] = true
	return nil
}

func (h *Hub) unsubscribe(c *client, channel string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.channels[channel], c)
	if len(h.channels[channel]) == 0 {
		delete(h.channels, channel)
	}
	delete(c.channels, channel)
}

// drop disconnects a client from every channel and closes its outbox.
func (h *Hub) drop(c *client) {
	h.mu.Lock()
	for channel := range c.channels {
		delete(h.channels[channel], c)
		if len(h.channels[channel]) == 0 {
			delete(h.channels, channel)
		}
	}
	c.channels = make(map[string]bool)
	h.mu.Unlock()
	c.close()
}

// Handler returns the Fiber handler that upgrades requests to
// WebSocket connections and serves the subscription protocol.
func (h *Hub) Handler() fiber.Handler {
	return websocket.New(func(conn *websocket.Conn) {
		h.serve(conn)
	})
}

func (h *Hub) serve(conn *websocket.Conn) {
	c := &client{
		send:     make(chan []byte, 64),
		channels: make(map[string]bool),
	}
	defer h.drop(c)

	// Writer: a single goroutine owns the connection's write side.
	done := make(chan struct{})
	go func() {
		defer close(done)
		for frame := range c.send {
			if err := conn.WriteMessage(websocket.TextMessage, frame); err != nil {
				return
			}
		}
	}()

	reply := func(m Message) {
		frame, _ := json.Marshal(m)
		c.trySend(frame)
	}

	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			break
		}
		var frame inbound
		if err := json.Unmarshal(raw, &frame); err != nil {
			reply(Message{Event: "genesys:error", Payload: "malformed frame"})
			continue
		}

		switch frame.Action {
		case "subscribe":
			if err := h.subscribe(c, frame.Channel, frame.Auth); err != nil {
				reply(Message{Channel: frame.Channel, Event: "genesys:error", Payload: err.Error()})
				continue
			}
			reply(Message{Channel: frame.Channel, Event: "genesys:subscribed"})
		case "unsubscribe":
			h.unsubscribe(c, frame.Channel)
			reply(Message{Channel: frame.Channel, Event: "genesys:unsubscribed"})
		case "ping":
			reply(Message{Event: "genesys:pong"})
		default:
			reply(Message{Event: "genesys:error", Payload: "unknown action"})
		}
	}

	// Reader finished: stop the writer and wait for it.
	c.close()
	<-done
}
