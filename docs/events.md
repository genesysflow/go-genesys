# Events

## Typed events (recommended)

Any struct can be an event; the name is derived from its type:

```go
type OrderShipped struct {
    OrderID int
}

// Listen with full type safety — no assertions
events.Listen(dispatcher, func(e *OrderShipped) error {
    return notifyCustomer(e.OrderID)
})

// Dispatch
events.Emit(dispatcher, &OrderShipped{OrderID: 7})
```

Custom names are supported by implementing `events.Event`:

```go
func (e *OrderShipped) Name() string { return "orders.shipped" }
```

## Name-based API

```go
dispatcher.Listen("orders.shipped", func(event events.Event) error {
    e := event.(*OrderShipped)
    ...
})
dispatcher.Dispatch(&OrderShipped{OrderID: 7})
dispatcher.HasListeners("orders.shipped")
dispatcher.Forget("orders.shipped")
```

## Facade

```go
import eventfacade "github.com/genesysflow/go-genesys/facades/event"

eventfacade.Dispatch(&OrderShipped{OrderID: 7})
events.Listen(eventfacade.Dispatcher(), handleOrderShipped)
```

Listener errors stop propagation and are returned to the dispatcher's
caller; keep listeners idempotent if you retry dispatches.
