package broadcast

import (
	"context"
	"encoding/json"

	"github.com/genesysflow/go-genesys/support"
	"github.com/redis/go-redis/v9"
)

// backplaneEnvelope wraps a relayed message with its origin so a hub
// never re-delivers its own publishes.
type backplaneEnvelope struct {
	Origin  string  `json:"origin"`
	Message Message `json:"message"`
}

// ConnectRedis joins this hub to a Redis pub/sub backplane, so
// Broadcast calls on any connected instance reach every instance's
// local subscribers - multi-node broadcasting for the in-memory hub.
// The returned stop function detaches the hub and closes the
// subscription:
//
//	stop, err := hub.ConnectRedis(client, "genesys:broadcast")
//	defer stop()
func (h *Hub) ConnectRedis(client *redis.Client, channelName string) (func(), error) {
	if channelName == "" {
		channelName = "genesys:broadcast"
	}
	origin := support.RandomString(16)
	ctx, cancel := context.WithCancel(context.Background())

	pubsub := client.Subscribe(ctx, channelName)
	// Force the subscription to be established before we return, so a
	// Broadcast right after ConnectRedis is not lost.
	if _, err := pubsub.Receive(ctx); err != nil {
		cancel()
		pubsub.Close()
		return nil, err
	}

	h.mu.Lock()
	h.backplane = func(message Message) error {
		encoded, err := json.Marshal(backplaneEnvelope{Origin: origin, Message: message})
		if err != nil {
			return err
		}
		return client.Publish(ctx, channelName, encoded).Err()
	}
	h.mu.Unlock()

	go func() {
		for raw := range pubsub.Channel() {
			var envelope backplaneEnvelope
			if err := json.Unmarshal([]byte(raw.Payload), &envelope); err != nil {
				continue
			}
			if envelope.Origin == origin {
				continue // our own publish; already delivered locally
			}
			_ = h.deliverLocal(envelope.Message)
		}
	}()

	return func() {
		h.mu.Lock()
		h.backplane = nil
		h.mu.Unlock()
		cancel()
		pubsub.Close()
	}, nil
}
