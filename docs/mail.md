# Mail

## Configuration

```yaml
# config/mail.yaml
driver: log             # smtp, log, array
host: smtp.example.com
port: 587
username: ${MAIL_USERNAME:-}
password: ${MAIL_PASSWORD:-}
encryption: starttls    # tls (implicit, port 465), starttls, none
from_address: hello@example.com
from_name: My App
```

The `log` driver (default) writes rendered messages to the application log —
nothing leaves the machine. The `array` driver captures messages in memory
for assertions in tests.

## Sending

```go
import mailfacade "github.com/genesysflow/go-genesys/facades/mail"

message := mailfacade.Message().
    To("user@example.com").
    Cc("audit@example.com").
    Subject("Welcome!").
    HTML("<h1>Hello!</h1>").
    Text("Hello!").
    Attach("invoice.pdf", pdfBytes, "application/pdf")

if err := mailfacade.Send(message); err != nil {
    return err
}
```

The sender defaults to `from_address`/`from_name`; override per message
with `.From(address, name)`. Bcc recipients receive the message but are
never rendered into headers.

## Rendering bodies with views

```go
html, err := viewfacade.Render("emails.welcome", map[string]any{"name": user.Name})
message := mailfacade.Message().To(user.Email).Subject("Welcome").HTML(html)
```

## Testing

```go
mailer := mail.NewArrayMailer(mail.Config{FromAddress: "test@app"})
svc := NewWelcomeService(mailer)
svc.SendWelcome(user)

sent := mailer.Sent()
assert.Len(t, sent, 1)
assert.Equal(t, "Welcome!", sent[0].GetSubject())
```

## Templated Emails

Render mail bodies through the view layer:

```go
message := mail.NewMessage().
    To(user.Email).
    Subject("Welcome!").
    ViewHTML(views, "emails.welcome", map[string]any{"name": user.Name})
```

`ViewText` renders a plain-text body the same way; render errors are
deferred and surface from `Send`.

## Mailables

An email as an object, rather than a message built at the call site:

```go
type OrderShipped struct{ Order *models.Order }

func (m *OrderShipped) Envelope() mail.Envelope {
    return mail.Envelope{
        From:    "shop@example.com",
        Subject: "Your order has shipped",
    }
}

func (m *OrderShipped) Content() mail.Content {
    return mail.Content{
        View: "emails.shipped",
        Data: map[string]any{"order": m.Order},
    }
}

// Optional:
func (m *OrderShipped) Attachments() []mail.Attachment {
    return []mail.Attachment{{Filename: "invoice.pdf", Content: pdf, ContentType: "application/pdf"}}
}
```

Sending:

```go
mail.SendDefault(&OrderShipped{Order: order})                        // the application's mailer
mail.Send(mailer, &OrderShipped{Order: order})                       // an explicit mailer
mail.SendWith(mailer, views, &OrderShipped{Order: order})            // renders Content.View
mail.To(mailer, user.Email).Using(views).Send(&OrderShipped{...})    // recipient at send time
```

`SendDefault` resolves the mailer when the message is sent rather than
when the handler was wired, which is usually what a handler wants: it
has a mailable and no mailer to hand, and capturing one at wiring time
is what stops a test's array mailer from ever seeing the message.

A mailable naming a view with no renderer, or whose view fails to render,
reports the error rather than delivering a blank email.
