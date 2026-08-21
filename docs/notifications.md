# Notifications

Notifications deliver a single message through multiple channels — an
email and a database record for an in-app feed, from one definition.

## Defining a notification

A notification declares its channels with `Via` and provides per-channel
content with `ToMail` / `ToDatabase`:

```go
type InvoicePaid struct{ Amount int }

func (n *InvoicePaid) Via(_ notifications.Notifiable) []string {
    return []string{"mail", "database"}
}

func (n *InvoicePaid) ToMail(_ notifications.Notifiable) *mail.Message {
    return mail.NewMessage().Subject("Invoice paid").Text("Thanks!")
}

func (n *InvoicePaid) ToDatabase(_ notifications.Notifiable) map[string]any {
    return map[string]any{"amount": n.Amount}
}
```

## Receiving notifications

Anything can receive notifications by routing each channel:

```go
func (u *User) RouteNotificationFor(channel string) any {
    switch channel {
    case "mail":
        return u.Email
    case "database":
        return u.ID
    }
    return nil // returning nil skips the channel
}
```

Send through the manager (or the `notify` facade once the
`NotificationServiceProvider` is registered):

```go
notify.Send(user, &InvoicePaid{Amount: 100})

// One-off address without a model:
notify.Send(notifications.Route("mail", "ops@example.com"), &ServerDown{})
```

## The database channel

The database channel stores JSON payloads in a `notifications` table:

```sql
CREATE TABLE notifications (
    id            INTEGER PRIMARY KEY,
    notifiable_id TEXT      NOT NULL,
    name          TEXT      NOT NULL,
    data          TEXT      NOT NULL,
    read_at       TIMESTAMP,
    created_at    TIMESTAMP NOT NULL
);
```

Build feeds with the read/unread helpers:

```go
items, _ := notify.UnreadFor(user.ID)   // newest first
notify.MarkAsRead(items[0].ID)
notify.MarkAllAsRead(user.ID)
```

The stored `name` defaults to the snake_cased type name
(`invoice_paid`); implement `NotificationName() string` to override it.

## Queued Notifications

`SendQueued` resolves each channel's route (email address, database id)
at dispatch time and pushes a serializable job, so any worker process
can deliver it. Register the notification type once:

```go
notifications.RegisterQueued[InvoicePaid]()

manager.SendQueued(q, user, &InvoicePaid{Amount: 100})
// ... a worker later delivers on every routed channel
```

Workers deliver through the manager installed by the
NotificationServiceProvider (`notifications.SetDefault`).
