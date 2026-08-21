package providers_test

import (
	"testing"

	"github.com/genesysflow/go-genesys/broadcast"
	"github.com/genesysflow/go-genesys/container"
	"github.com/genesysflow/go-genesys/foundation"
	"github.com/genesysflow/go-genesys/notifications"
	"github.com/genesysflow/go-genesys/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type deployed struct{ Version string }

func (n *deployed) Via(notifiable notifications.Notifiable) []string { return []string{"broadcast"} }

func (n *deployed) ToBroadcast(notifiable notifications.Notifiable) notifications.BroadcastMessage {
	return notifications.BroadcastMessage{Event: "deployed", Payload: map[string]any{"version": n.Version}}
}

// The broadcast channel is only usable if the provider hands the manager
// the application's hub.
func TestNotificationProviderWiresBroadcaster(t *testing.T) {
	app := foundation.New()
	hub := broadcast.New()
	app.InstanceType(hub)

	require.NoError(t, app.Register(&providers.NotificationServiceProvider{}))
	require.NoError(t, app.Boot())

	manager := container.MustResolve[*notifications.Manager](app)

	// Nobody is subscribed, but a wired broadcaster accepts the send;
	// an unwired one reports that no broadcaster is configured.
	err := manager.Send(notifications.Route("broadcast", "ops"), &deployed{Version: "2.0"})
	assert.NoError(t, err)
}
