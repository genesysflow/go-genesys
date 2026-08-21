# Broadcasting

Broadcasting pushes server-side events to connected clients over
WebSockets — Laravel's broadcasting with an in-memory hub. Each process
serves its own connections; put instances behind sticky sessions when
scaling out.

## Serving connections

```go
hub := broadcast.New()
kernel.Fiber().Get("/ws", hub.Handler())
```

Clients speak a small JSON protocol:

```json
-> {"action": "subscribe", "channel": "orders"}
<- {"channel": "orders", "event": "genesys:subscribed"}
<- {"channel": "orders", "event": "order.shipped", "payload": {"id": 42}}
-> {"action": "unsubscribe", "channel": "orders"}
-> {"action": "ping"}
```

## Broadcasting events

Directly:

```go
hub.Broadcast("orders", "order.shipped", map[string]any{"id": 42})
```

Or bridge the event dispatcher — any dispatched event implementing
`Broadcastable` goes out automatically:

```go
type OrderShipped struct{ ID int64 `json:"id"` }

func (e *OrderShipped) Name() string             { return "order.shipped" }
func (e *OrderShipped) BroadcastChannel() string { return "orders" }

broadcast.ConnectDispatcher(dispatcher, hub)
dispatcher.Dispatch(&OrderShipped{ID: 42}) // listeners run AND clients get the frame
```

`BroadcastAs()` overrides the event name; `BroadcastPayload()` overrides
the payload (defaults to the JSON encoding of the event).

## Private channels

Channels prefixed `private-` require authorization. Install an
authorizer that checks the token the client sends with its subscribe
frame — for example a signed token minted per user:

```go
hub.Authorize(func(channel, token string) bool {
    userID, err := verifyChannelToken(token)
    return err == nil && channel == fmt.Sprintf("private-user.%d", userID)
})
```

Slow clients whose outboxes fill up are disconnected rather than
blocking a broadcast; reconnecting is the client's job.
