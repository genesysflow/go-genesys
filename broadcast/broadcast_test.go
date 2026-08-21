package broadcast_test

import (
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/genesysflow/go-genesys/broadcast"
	"github.com/genesysflow/go-genesys/events"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// wsClient wraps a raw connection with frame helpers.
type wsClient struct {
	t    *testing.T
	conn *websocket.Conn
}

func startHub(t *testing.T) (*broadcast.Hub, string) {
	t.Helper()
	hub := broadcast.New()
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Get("/ws", hub.Handler())

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	go app.Listener(listener)
	t.Cleanup(func() { app.Shutdown() })

	return hub, "ws://" + listener.Addr().String() + "/ws"
}

func dial(t *testing.T, url string) *wsClient {
	t.Helper()
	var conn *websocket.Conn
	var err error
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		conn, _, err = websocket.DefaultDialer.Dial(url, nil)
		if err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })
	return &wsClient{t: t, conn: conn}
}

func (c *wsClient) send(frame map[string]any) {
	c.t.Helper()
	require.NoError(c.t, c.conn.WriteJSON(frame))
}

func (c *wsClient) read() broadcast.Message {
	c.t.Helper()
	c.conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, raw, err := c.conn.ReadMessage()
	require.NoError(c.t, err)
	var message broadcast.Message
	require.NoError(c.t, json.Unmarshal(raw, &message))
	return message
}

func (c *wsClient) subscribe(channel string, auth ...string) {
	c.t.Helper()
	frame := map[string]any{"action": "subscribe", "channel": channel}
	if len(auth) > 0 {
		frame["auth"] = auth[0]
	}
	c.send(frame)
	reply := c.read()
	require.Equal(c.t, "genesys:subscribed", reply.Event, "subscription failed: %v", reply.Payload)
}

func waitForSubscribers(t *testing.T, hub *broadcast.Hub, channel string, n int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for hub.Subscribers(channel) < n && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	require.Equal(t, n, hub.Subscribers(channel))
}

func TestPublicChannelBroadcast(t *testing.T) {
	hub, url := startHub(t)

	alice := dial(t, url)
	bob := dial(t, url)
	alice.subscribe("orders")
	bob.subscribe("orders")
	waitForSubscribers(t, hub, "orders", 2)

	require.NoError(t, hub.Broadcast("orders", "order.shipped", map[string]any{"id": 42}))

	for _, c := range []*wsClient{alice, bob} {
		message := c.read()
		assert.Equal(t, "orders", message.Channel)
		assert.Equal(t, "order.shipped", message.Event)
		payload := message.Payload.(map[string]any)
		assert.EqualValues(t, 42, payload["id"])
	}
}

func TestChannelsAreIsolated(t *testing.T) {
	hub, url := startHub(t)

	orders := dial(t, url)
	orders.subscribe("orders")
	waitForSubscribers(t, hub, "orders", 1)

	// Broadcast to another channel, then to orders; the client must
	// only ever see the orders frame.
	require.NoError(t, hub.Broadcast("payments", "paid", nil))
	require.NoError(t, hub.Broadcast("orders", "order.created", nil))

	message := orders.read()
	assert.Equal(t, "orders", message.Channel)
	assert.Equal(t, "order.created", message.Event)
}

func TestPrivateChannelAuthorization(t *testing.T) {
	hub, url := startHub(t)
	hub.Authorize(func(channel, token string) bool {
		return token == "valid-token"
	})

	c := dial(t, url)

	// Wrong token: rejected.
	c.send(map[string]any{"action": "subscribe", "channel": "private-user.1", "auth": "bad"})
	reply := c.read()
	assert.Equal(t, "genesys:error", reply.Event)
	assert.Equal(t, 0, hub.Subscribers("private-user.1"))

	// Right token: accepted and receiving.
	c.subscribe("private-user.1", "valid-token")
	waitForSubscribers(t, hub, "private-user.1", 1)
	require.NoError(t, hub.Broadcast("private-user.1", "secret", "shh"))
	message := c.read()
	assert.Equal(t, "secret", message.Event)
}

func TestUnsubscribeAndDisconnectCleanUp(t *testing.T) {
	hub, url := startHub(t)

	c := dial(t, url)
	c.subscribe("orders")
	waitForSubscribers(t, hub, "orders", 1)

	c.send(map[string]any{"action": "unsubscribe", "channel": "orders"})
	reply := c.read()
	assert.Equal(t, "genesys:unsubscribed", reply.Event)
	waitForSubscribers(t, hub, "orders", 0)

	// Disconnecting removes remaining subscriptions.
	c.subscribe("payments")
	waitForSubscribers(t, hub, "payments", 1)
	c.conn.Close()
	deadline := time.Now().Add(2 * time.Second)
	for hub.Subscribers("payments") > 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	assert.Equal(t, 0, hub.Subscribers("payments"))
}

// --- event bridge ---

type orderShipped struct {
	ID int64 `json:"id"`
}

func (e *orderShipped) Name() string             { return "order.shipped" }
func (e *orderShipped) BroadcastChannel() string { return "orders" }

type quietEvent struct{}

func (e *quietEvent) Name() string { return "internal.only" }

func TestDispatcherBridge(t *testing.T) {
	hub, url := startHub(t)
	dispatcher := events.NewDispatcher()
	broadcast.ConnectDispatcher(dispatcher, hub)

	c := dial(t, url)
	c.subscribe("orders")
	waitForSubscribers(t, hub, "orders", 1)

	// Non-broadcastable events pass through silently.
	require.NoError(t, dispatcher.Dispatch(&quietEvent{}))
	// Broadcastable events reach the channel.
	require.NoError(t, dispatcher.Dispatch(&orderShipped{ID: 7}))

	message := c.read()
	assert.Equal(t, "order.shipped", message.Event)
	payload := message.Payload.(map[string]any)
	assert.EqualValues(t, 7, payload["id"])
}
