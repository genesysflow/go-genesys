package broadcast_test

import (
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPresenceChannelMembership(t *testing.T) {
	hub, url := startHub(t)
	hub.Authorize(func(channel, token string) bool { return token == "ok" })

	ada := dial(t, url)
	ada.send(map[string]any{
		"action": "subscribe", "channel": "presence-room", "auth": "ok",
		"info": map[string]any{"name": "Ada"},
	})
	require.Equal(t, "genesys:subscribed", ada.read().Event)
	here := ada.read()
	require.Equal(t, "genesys:here", here.Event)
	assert.Len(t, here.Payload.([]any), 1, "Ada sees herself in the member list")

	bob := dial(t, url)
	bob.send(map[string]any{
		"action": "subscribe", "channel": "presence-room", "auth": "ok",
		"info": map[string]any{"name": "Bob"},
	})
	require.Equal(t, "genesys:subscribed", bob.read().Event)
	bobHere := bob.read()
	require.Equal(t, "genesys:here", bobHere.Event)
	assert.Len(t, bobHere.Payload.([]any), 2, "Bob sees both members")

	// Ada learns Bob joined.
	joining := ada.read()
	require.Equal(t, "genesys:joining", joining.Event)
	assert.Equal(t, "Bob", joining.Payload.(map[string]any)["name"])

	assert.Len(t, hub.Members("presence-room"), 2)

	// Bob leaves; Ada hears it.
	bob.send(map[string]any{"action": "unsubscribe", "channel": "presence-room"})
	require.Equal(t, "genesys:unsubscribed", bob.read().Event)
	leaving := ada.read()
	require.Equal(t, "genesys:leaving", leaving.Event)
	assert.Equal(t, "Bob", leaving.Payload.(map[string]any)["name"])
}

func TestPresenceRequiresAuth(t *testing.T) {
	hub, url := startHub(t)
	hub.Authorize(func(channel, token string) bool { return token == "ok" })

	c := dial(t, url)
	c.send(map[string]any{"action": "subscribe", "channel": "presence-room", "auth": "wrong"})
	assert.Equal(t, "genesys:error", c.read().Event)
}

func TestRedisBackplaneRelaysAcrossHubs(t *testing.T) {
	mr := miniredis.RunT(t)
	clientA := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	clientB := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { clientA.Close(); clientB.Close() })

	hubA, urlA := startHub(t)
	hubB, urlB := startHub(t)

	stopA, err := hubA.ConnectRedis(clientA, "test:broadcast")
	require.NoError(t, err)
	t.Cleanup(stopA)
	stopB, err := hubB.ConnectRedis(clientB, "test:broadcast")
	require.NoError(t, err)
	t.Cleanup(stopB)

	onA := dial(t, urlA)
	onA.subscribe("orders")
	onB := dial(t, urlB)
	onB.subscribe("orders")
	waitForSubscribers(t, hubA, "orders", 1)
	waitForSubscribers(t, hubB, "orders", 1)

	// A broadcast on hub A reaches subscribers of BOTH hubs, once each.
	require.NoError(t, hubA.Broadcast("orders", "order.shipped", map[string]any{"id": 7}))

	msgA := onA.read()
	assert.Equal(t, "order.shipped", msgA.Event)
	msgB := onB.read()
	assert.Equal(t, "order.shipped", msgB.Event, "the backplane relayed to the other hub")
	assert.EqualValues(t, 7, msgB.Payload.(map[string]any)["id"])
}
