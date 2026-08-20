# Mail

## Configuration

```yaml
# config/mail.yaml
driver: log             # smtp, log, array
host: smtp.example.com
port: 587
username: ${MAIL_USERNAME:}
password: ${MAIL_PASSWORD:}
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
