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
	Action  string          `json:"action"`
	Channel string          `json:"channel"`
	Auth    string          `json:"auth,omitempty"`
	Info    json.RawMessage `json:"info,omitempty"` // presence member info
}

// Authorizer decides whether a token may join a private channel.
type Authorizer func(channel, token string) bool

// Hub tracks channels and their subscribers.
type Hub struct {
	mu        sync.RWMutex
	channels  map[string]map[*client]bool
	members   map[string]map[*client]json.RawMessage // presence channels
	authorize Authorizer

	// backplane, when set, relays Broadcast frames to other hub
	// instances (see ConnectRedis).
	backplane func(Message) error
}

// client is one connected WebSocket peer.
type client struct {
	sendMu   sync.Mutex
	closed   bool
	send     chan []byte
	channels map[string]bool
	conn     interface{ Close() error }
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
	return &Hub{
		channels: make(map[string]map[*client]bool),
		members:  make(map[string]map[*client]json.RawMessage),
	}
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

// IsPresence reports whether a channel is a presence channel: it
// requires authorization AND tracks who is subscribed. Joining clients
// receive a genesys:here member list; everyone else receives
// genesys:joining / genesys:leaving as membership changes.
func IsPresence(channel string) bool {
	return strings.HasPrefix(channel, "presence-")
}

// Broadcast sends an event to every subscriber of a channel. Slow
// clients whose buffers are full are disconnected rather than blocking
// the broadcast. When a backplane is connected (ConnectRedis) the frame
// is also relayed to the other hub instances.
func (h *Hub) Broadcast(channel, event string, payload any) error {
	message := Message{Channel: channel, Event: event, Payload: payload}
	if err := h.deliverLocal(message); err != nil {
		return err
	}
	h.mu.RLock()
	backplane := h.backplane
	h.mu.RUnlock()
	if backplane != nil {
		return backplane(message)
	}
	return nil
}

// deliverLocal fans a message out to this instance's subscribers.
func (h *Hub) deliverLocal(message Message) error {
	frame, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("broadcast: cannot encode payload: %w", err)
	}
	h.deliverFrame(message.Channel, frame, nil)
	return nil
}

// deliverFrame sends a pre-encoded frame to a channel's subscribers,
// skipping one client (the originator of a membership event).
func (h *Hub) deliverFrame(channel string, frame []byte, skip *client) {
	h.mu.RLock()
	subscribers := make([]*client, 0, len(h.channels[channel]))
	for c := range h.channels[channel] {
		if c != skip {
			subscribers = append(subscribers, c)
		}
	}
	h.mu.RUnlock()

	for _, c := range subscribers {
		if !c.trySend(frame) {
			h.drop(c) // slow or closed consumer
		}
	}
}

// Subscribers returns how many clients are on a channel.
func (h *Hub) Subscribers(channel string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.channels[channel])
}

func (h *Hub) subscribe(c *client, channel, token string, info json.RawMessage) error {
	if IsPrivate(channel) || IsPresence(channel) {
		h.mu.RLock()
		authorize := h.authorize
		h.mu.RUnlock()
		if authorize == nil || !authorize(channel, token) {
			return fmt.Errorf("unauthorized for channel %s", channel)
		}
	}

	h.mu.Lock()
	if h.channels[channel] == nil {
		h.channels[channel] = make(map[*client]bool)
	}
	h.channels[channel][c] = true
	c.channels[channel] = true
	if IsPresence(channel) {
		if info == nil {
			info = json.RawMessage(`{}`)
		}
		if h.members[channel] == nil {
			h.members[channel] = make(map[*client]json.RawMessage)
		}
		h.members[channel][c] = info
	}
	h.mu.Unlock()
	return nil
}

// announcePresence sends the joiner the current member list and tells
// everyone else who joined; called after the subscribed ack so frame
// order is predictable.
func (h *Hub) announcePresence(c *client, channel string) {
	if !IsPresence(channel) {
		return
	}
	h.mu.RLock()
	info := h.members[channel][c]
	here := make([]json.RawMessage, 0, len(h.members[channel]))
	for _, memberInfo := range h.members[channel] {
		here = append(here, memberInfo)
	}
	h.mu.RUnlock()

	if frame, err := json.Marshal(Message{Channel: channel, Event: "genesys:here", Payload: here}); err == nil {
		c.trySend(frame)
	}
	if frame, err := json.Marshal(Message{Channel: channel, Event: "genesys:joining", Payload: info}); err == nil {
		h.deliverFrame(channel, frame, c)
	}
}

// Members returns the presence member info currently on a channel.
func (h *Hub) Members(channel string) []json.RawMessage {
	h.mu.RLock()
	defer h.mu.RUnlock()
	members := make([]json.RawMessage, 0, len(h.members[channel]))
	for _, info := range h.members[channel] {
		members = append(members, info)
	}
	return members
}

func (h *Hub) unsubscribe(c *client, channel string) {
	h.mu.Lock()
	info, wasMember := h.members[channel][c]
	delete(h.members[channel], c)
	if len(h.members[channel]) == 0 {
		delete(h.members, channel)
	}
	delete(h.channels[channel], c)
	if len(h.channels[channel]) == 0 {
		delete(h.channels, channel)
	}
	delete(c.channels, channel)
	h.mu.Unlock()

	if wasMember {
		if frame, err := json.Marshal(Message{Channel: channel, Event: "genesys:leaving", Payload: info}); err == nil {
			h.deliverFrame(channel, frame, c)
		}
	}
}

// drop disconnects a client from every channel, closes its outbox, and
// closes the underlying connection so the peer learns it was dropped
// instead of lingering as a subscribed-looking zombie. Presence
// channels see the member leave.
func (h *Hub) drop(c *client) {
	type departure struct {
		channel string
		info    json.RawMessage
	}
	var departures []departure

	h.mu.Lock()
	for channel := range c.channels {
		if info, ok := h.members[channel][c]; ok {
			departures = append(departures, departure{channel: channel, info: info})
			delete(h.members[channel], c)
			if len(h.members[channel]) == 0 {
				delete(h.members, channel)
			}
		}
		delete(h.channels[channel], c)
		if len(h.channels[channel]) == 0 {
			delete(h.channels, channel)
		}
	}
	c.channels = make(map[string]bool)
	h.mu.Unlock()
	c.close()
	if c.conn != nil {
		_ = c.conn.Close() // unblocks the read loop; safe on repeat drops
	}

	for _, d := range departures {
		if frame, err := json.Marshal(Message{Channel: d.channel, Event: "genesys:leaving", Payload: d.info}); err == nil {
			h.deliverFrame(d.channel, frame, c)
		}
	}
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
		conn:     conn,
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
			if err := h.subscribe(c, frame.Channel, frame.Auth, frame.Info); err != nil {
				reply(Message{Channel: frame.Channel, Event: "genesys:error", Payload: err.Error()})
				continue
			}
			reply(Message{Channel: frame.Channel, Event: "genesys:subscribed"})
			h.announcePresence(c, frame.Channel)
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
