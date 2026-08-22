# Testing

Go-Genesys ships a Laravel-style HTTP test DSL and fakes for the queue,
events, and mail, so feature tests read like the code they exercise.

## HTTP tests

`http.NewTestCase` drives requests through a kernel and asserts on the
response:

```go
func TestUsersEndpoint(t *testing.T) {
    kernel := setupApp(t)
    tc := http.NewTestCase(t, kernel)

    tc.Get("/users").
        AssertOK().
        AssertJsonPath("data.0.name", "Ada").
        AssertJsonCount("data", 2)

    tc.Post("/users", map[string]any{"name": "Carol"}).
        AssertCreated().
        AssertJsonPath("name", "Carol")

    tc.Post("/users", map[string]any{}).
        AssertValidationError("name")

    tc.WithToken("secret").Get("/me").AssertOK()
}
```

Available assertions: `AssertStatus/OK/Created/NoContent/BadRequest/
Unauthorized/Forbidden/NotFound/Unprocessable`, `AssertRedirect`,
`AssertHeader`, `AssertSee`/`AssertDontSee`, `AssertJson` (top-level
subset), `AssertJsonPath` (dot paths with array indices),
`AssertJsonCount`, and `AssertValidationError`. All are chainable.

`tc.PostForm` sends a form body, encoding keys and values, so a field
carrying a space or an ampersand arrives as it was written.

A browser states what it accepts, and the framework reads that to decide
between an HTML redirect and a JSON error body. A test driving an HTML
site has to say the same thing, or it exercises the API's behaviour by
accident:

```go
tc := http.NewTestCase(t, kernel).WithHeader("Accept", "text/html")
```

The most useful HTTP test boots the real application - every provider,
the real routes, the real middleware - against a throwaway database,
rather than assembling a kernel of its own. A test that wires the
application up itself can pass on wiring the application does not have.
`example/tests/harness_test.go` is a worked example.

## Queue fakes

`queue.NewFake` records dispatches without running jobs:

```go
fake := queue.NewFake()
manager.Register("sync", fake) // or swap via the facade

svc.RegisterUser(...)

queue.AssertPushed[SendWelcomeEmail](t, fake, func(j *SendWelcomeEmail) bool {
    return j.Email == "ada@example.com"
})
queue.AssertNotPushed[ChargeCard](t, fake)
fake.AssertPushedCount(t, 1)
```

## Event fakes

`dispatcher.Fake()` suppresses listeners and records what fired:

```go
dispatcher.Fake()

svc.PlaceOrder(...)

dispatcher.AssertDispatched(t, "order.placed")
events.AssertDispatchedEvent[*OrderPlaced](t, dispatcher, func(e *OrderPlaced) bool {
    return e.Total == 100
})
dispatcher.Unfake() // resume delivery
```

## Mail fakes

The array mailer captures messages and asserts on them:

```go
mailer := mail.NewArrayMailer(mail.Config{FromAddress: "app@example.com"})

svc.SendReceipt(...)

mailer.AssertSentTo(t, "ada@example.com")
mailer.AssertSent(t, func(m *mail.Message) bool {
    return m.GetSubject() == "Receipt"
})
```

## Fake data

`support/faker` generates realistic values for factories and seeders,
deterministic when seeded:

```go
fake := faker.NewSeeded(42)

userFactory := database.NewFactory(func(i int) *User {
    return &User{Name: fake.Name(), Email: fake.Email()}
})
users, _ := userFactory.Create(50)
```

## Authenticating a test

```go
tc := http.NewTestCase(t, kernel)

tc.ActingAs(&models.User{ID: 1}).Get("/dashboard").AssertOK()
tc.ActingAsGuest().Get("/dashboard").AssertRedirect("/login")
```

## Sessions across requests

The test case keeps a cookie jar, so a session survives a redirect the
way a browser's would:

```go
tc.Post("/login", map[string]string{"email": "...", "password": "..."}).AssertRedirect("/")
tc.Get("/dashboard").AssertOK()   // same session

tc.FlushCookies()                 // isolate what follows
```

## Database assertions

```go
import "github.com/genesysflow/go-genesys/testutil/dbtest"

dbtest.AssertDatabaseHas(t, "users", map[string]any{"email": "ada@example.com"})
dbtest.AssertDatabaseMissing(t, "users", map[string]any{"email": "gone@example.com"})
dbtest.AssertDatabaseCount(t, "users", 3)
dbtest.AssertSoftDeleted(t, "posts", map[string]any{"id": post.ID})
dbtest.AssertNotSoftDeleted(t, "posts", map[string]any{"id": post.ID})
```

A missing table or connection is reported rather than read as "no
matching row", which would turn a setup mistake into a passing assertion.

## More response assertions

```go
tc.Get("/me").
    AssertOK().
    AssertJsonPath("data.email", "ada@example.com").
    AssertJsonMissing("data.password").
    AssertHeaderMissing("X-Debug").
    AssertCookie("genesys_session", tc.Cookie("genesys_session")).
    AssertLocationContains("/users/")
```
