# Go-Genesys

A Laravel-inspired web framework for Go, providing elegant syntax and powerful features for building modern web applications.

## Features

- **Service Container & Providers**: Dependency injection with a clean register/boot lifecycle
- **HTTP Layer**: Built on [Fiber](https://github.com/gofiber/fiber) with middleware pipeline, route groups, named routes, fallbacks, redirects, and middleware groups/aliases
- **Query Builder**: Fluent SQL builder with per-driver grammar (`db.Table("users").Where(...).Get()`)
- **ORM**: Generics-based model layer — `database.Find[User](1)`, typed queries, automatic timestamps, pagination, relationships with batched eager loading (`With("Posts.Tags")`), soft deletes, lifecycle hooks/observers, and dirty tracking with partial updates
- **Route Model Binding**: `http.BindModel[User](ctx, "user")` with automatic 404s
- **Form Requests**: `http.ValidateRequest[T](ctx)` with Laravel-shaped 422 responses
- **Migrations & Schema Builder**: Programmatic schema with foreign keys, indexes, rollback/reset/fresh
- **Seeders & Factories**: `db:seed`, `database.NewFactory[T]`
- **Views**: `html/template`-based view layer with layouts, partials, and `ctx.View("users.index", data)`
- **Authentication**: Session and token guards, ORM user provider, login/logout/attempt, remember-me cookies, password reset broker, and signed email verification
- **Authorization**: Gate with `Define/Allows/Authorize` and before-hooks
- **Sessions**: Memory, file, database, and Redis drivers; flash data and old input
- **Cache**: Memory, file, and Redis stores with `Remember`, `Increment`, `Add`, `Pull`, atomic locks (`cache.NewLock`), and a cache-backed `Throttle` rate limiter
- **Queue**: Sync, memory, database, and Redis drivers; workers with retries/backoff, failed-job management, job chaining (`queue.Chain`), and batches with `Then/Catch/Finally`
- **Task Scheduling**: Cron-style scheduler with `schedule:run` / `schedule:work`
- **Events**: Dispatcher with typed generic listeners (`events.Listen[T]`)
- **Mail**: Message builder with SMTP, log, and array (test) drivers
- **Notifications**: Multi-channel notifications (mail + database) with on-demand routes and unread feeds
- **Encryption**: AES-256-GCM `crypt` package keyed by `APP_KEY`, plus `key:generate`
- **Signed URLs**: HMAC-signed links with optional expiry and a validation middleware
- **HTTP Client**: Fluent outbound client with retries (`client.New().WithToken(...).Get(...)`)
- **Localization**: Per-locale YAML lang files, `Trans`/`TransChoice`
- **Collections**: Generic fluent collections (`Map`, `Filter`, `GroupBy`, ...)
- **Validation**: Struct-tag validation with custom rules and messages
- **Security**: CSRF protection, bcrypt hashing, security headers, secure-by-default cookies
- **Filesystem**: Unified storage abstraction (local, S3)
- **Logging**: Structured logging with multiple channels
- **Console**: Rich CLI — `migrate`, `queue:work`, `schedule:work`, `route:list`, `about`, `db:seed`, `key:generate`, and a full set of `make:*` generators
- **Facades**: Static accessors for every core service (`db`, `cache`, `queue`, `auth`, `mail`, ...)
- **Testing**: HTTP test DSL (`http.NewTestCase`) with Laravel-style assertions, plus queue/event/mail fakes
- **Maintenance Mode**: `genesys down`/`up` with secret bypass and 503 + Retry-After
- **DB Observability**: `manager.Listen` fires a QueryEvent (SQL, bindings, duration) for every query
- **Fake Data**: `support/faker` for factories and seeders (deterministic when seeded)

## Installation

```bash
go get github.com/genesysflow/go-genesys
```

## Quick Start

```go
package main

import (
    "github.com/genesysflow/go-genesys/container"
    "github.com/genesysflow/go-genesys/foundation"
    "github.com/genesysflow/go-genesys/http"
    "github.com/genesysflow/go-genesys/providers"
)

func main() {
    app := foundation.New()

    app.Register(&providers.AppServiceProvider{})
    app.Register(&providers.RouteServiceProvider{})

    app.Boot()

    kernel := container.MustResolve[*http.Kernel](app, "http.kernel")
    kernel.Run(":3000")
}
```

See [`example/`](example/) for a complete application and [`docs/`](docs/) for per-topic guides.

## Database

### Query Builder

```go
import "github.com/genesysflow/go-genesys/facades/db"

// All rows as maps
users, err := db.Table("users").Get()

// Fluent conditions
rows, err := db.Table("users").
    Where("age", ">", 18).
    WhereIn("role", "admin", "editor").
    OrderByDesc("created_at").
    Limit(10).
    Get()

// Joins, aggregates, pagination
count, err := db.Table("users").Where("active", true).Count()
page, err := db.Table("users").OrderBy("id").Paginate(2, 15)
```

### ORM

```go
type User struct {
    database.Model             // ID, CreatedAt, UpdatedAt
    Name  string `db:"name"  json:"name"`
    Email string `db:"email" json:"email"`
}

// CRUD
user := &User{Name: "Ada", Email: "ada@example.com"}
err := database.Create(user)        // sets ID + timestamps
found, err := database.Find[User](user.ID)
all, err := database.All[User]()
err = database.Delete[User](user.ID)

// Typed queries
adults, err := database.Query[User]().
    Where("age", ">=", 18).
    OrderBy("name").
    Get()

page, err := database.Query[User]().Latest().Paginate(1, 15)
```

Table names are inferred (`User` → `users`, `BlogPost` → `blog_posts`) and can be overridden with `TableName() string`.

### Migrations

```go
func (m *CreateUsersTable) Up(builder *schema.Builder) error {
    return builder.Create("users", func(table *schema.Blueprint) {
        table.ID()
        table.String("name", 255)
        table.String("email", 255).Unique()
        table.ForeignID("team_id")
        table.Foreign("team_id").CascadeOnDelete() // references teams.id
        table.Timestamps()
    })
}
```

### Seeders & Factories

```go
var userFactory = database.NewFactory(func(i int) *User {
    return &User{Name: fmt.Sprintf("User %d", i)}
})

// In a seeder registered via SeedServiceProvider:
users, err := userFactory.Create(10)
```

## HTTP

### Routing

```go
router.GET("/users/:user", ShowUser).Name("users.show")

router.Group("/admin", func(r *http.Router) {
    r.GET("/dashboard", Dashboard)
}, kernel.MiddlewareGroup("web")...)

router.Redirect("/old", "/new", 301)
router.Fallback(CustomNotFound)
```

### Form Requests & Resources

```go
type StoreUserRequest struct {
    Name  string `json:"name" validate:"required,min=3"`
    Email string `json:"email" validate:"required,email"`
}

func StoreUser(ctx *http.Context) error {
    req, err := http.ValidateRequest[StoreUserRequest](ctx)
    if err != nil {
        return err // 422 {"message": ..., "errors": {"name": [...]}}
    }
    user := &User{Name: req.Name, Email: req.Email}
    if err := database.Create(user); err != nil {
        return err
    }
    return ctx.Status(201).JSONResponse(map[string]any{"data": user})
}

// Route model binding
func ShowUser(ctx *http.Context) error {
    user, err := http.BindModel[User](ctx, "user") // 404 when missing
    if err != nil {
        return err
    }
    return ctx.Resource(user) // {"data": {...}}
}

// Pagination envelope: {"data": [...], "meta": {...}}
func ListUsers(ctx *http.Context) error {
    page, err := database.Query[User]().Paginate(ctx.QueryInt("page", 1), 15)
    if err != nil {
        return err
    }
    return ctx.Paginated(page)
}
```

### Views

```go
// resources/views/users/index.html renders as "users.index";
// compose with {{template "layouts.app" .}}
return ctx.View("users.index", map[string]any{"users": users})
```

## Authentication & Authorization

```go
provider := auth.NewORMUserProvider[User]()
guard := auth.NewSessionGuard("web", provider)

kernel.POST("/login", func(ctx *http.Context) error {
    user, err := guard.Attempt(ctx, map[string]any{
        "email":    ctx.Input("email"),
        "password": ctx.Input("password"),
    })
    if err != nil {
        return errors.Unauthorized("Invalid credentials")
    }
    return ctx.Resource(user)
})

kernel.GET("/profile", Profile, auth.Middleware(guard))

// Gates
gate.Define("update-post", func(user auth.Authenticatable, args ...any) bool {
    return args[0].(*Post).AuthorID == user.GetAuthIdentifier()
})
if err := gate.Authorize(user, "update-post", post); err != nil {
    return err // 403
}
```

## Cache

```go
import "github.com/genesysflow/go-genesys/facades/cache"

cache.Put("key", value, time.Minute)
value, err := cache.Get("key")
users, err := cache.Remember("users:all", time.Minute, func() (any, error) {
    return loadUsers()
})
n, err := cache.Increment("hits")
```

Stores are configured in `config/cache.yaml` (memory and file drivers built in).

## Queue

```go
// Define a job (exported fields are serialized)
type SendEmailJob struct {
    Email string `json:"email"`
}

func (j *SendEmailJob) Handle() error {
    return sendEmail(j.Email)
}

// Register once (required by workers), then dispatch
queue.Register[SendEmailJob]()
queuefacade.Dispatch(&SendEmailJob{Email: "user@example.com"})
queuefacade.DispatchLater(5*time.Minute, &SendEmailJob{Email: "user@example.com"})
```

Run workers with `genesys queue:work`; inspect failures with `queue:failed` and retry with `queue:retry <id|all>`. The database driver stores jobs durably (`queue.CreateJobsTables` sets up the tables).

## Scheduling

```go
app.Register(&providers.ScheduleServiceProvider{
    Define: func(s *schedule.Schedule) {
        s.Call(pruneSessions).Daily().Description("prune sessions")
        s.Call(sendReports).Cron("0 8 * * 1-5")
    },
})
```

Drive it with `schedule:work` (foreground loop) or `schedule:run` from system cron.

## Events

```go
type OrderShipped struct{ OrderID int }

events.Listen(dispatcher, func(e *OrderShipped) error {
    return notify(e.OrderID)
})

events.Emit(dispatcher, &OrderShipped{OrderID: 7})
```

## Mail

```go
message := mail.NewMessage().
    To("user@example.com").
    Subject("Welcome!").
    HTML("<h1>Hello</h1>").
    Text("Hello")

err := mailfacade.Send(message)
```

Drivers: `smtp` (TLS/STARTTLS), `log` (development default), `array` (tests).

## Security

### CSRF protection

Any application that authenticates with a cookie or session needs CSRF
protection on state-changing routes. Register the middleware with a secret that
is shared across every instance of the application:

```go
kernel.Use(middleware.CSRF(middleware.CSRFConfig{
    Secret: []byte(env.Require("APP_KEY")),
}))
```

Every other field falls back to a secure default, so a partial config is safe:
the cookie is marked `Secure`, and local development over plain `http://` opts
out with `CookieInsecure: true`.

Safe methods (GET, HEAD, OPTIONS, TRACE) mint a token; every other method must
echo it back in the `X-CSRF-Token` header or a `_token` form field. Render the
token from the context:

```go
kernel.GET("/form", func(ctx *http.Context) error {
    return ctx.HTML(renderForm(middleware.CSRFToken(ctx)))
})
```

Do not apply it to stateless, token-authenticated APIs — there is no ambient
credential for an attacker to abuse, so it only adds friction.

### Password hashing

Never store a plain digest of a password: SHA-256 and friends are built to be
fast, which is exactly what an offline cracker wants. The `hash` package wraps
bcrypt, which is deliberately slow and salts every hash:

```go
hashed, err := hash.Make(password)

if err := hash.Check(password, user.PasswordHash); err != nil {
    if errors.Is(err, hash.ErrMismatch) {
        return errors.Unauthorized("Invalid credentials")
    }
    return err
}

// On successful login — the one moment the plaintext is available — migrate
// older accounts to the current work factor.
if hash.NeedsRehash(user.PasswordHash) {
    user.PasswordHash, _ = hash.Make(password)
}
```

### Encryption & signed URLs

```go
// AES-256-GCM keyed by APP_KEY (genesys key:generate)
payload, err := crypt.EncryptString("secret")
plain, err := crypt.DecryptString(payload)

// Tamper-proof links with optional expiry
signer := urlsign.New(key)
link, err := signer.SignTemporary("https://app.test/unsubscribe?u=42", 24*time.Hour)
kernel.GET("/unsubscribe", handler, middleware.ValidateSignature(key))
```

### Security headers

```go
kernel.Use(middleware.Secure(middleware.SecureConfig{
    HSTSMaxAge:            31536000,
    HSTSIncludeSubdomains: true,
    ContentSecurityPolicy: "default-src 'self'",
}))
```

HSTS is off by default and must be enabled deliberately: turning it on for a
host that is not yet fully served over TLS locks clients out of it for the
lifetime of the `max-age`.

### Running behind a proxy

`Request.IP()` reports the connecting address and ignores forwarding headers
until you say which proxies to trust. Leave `TrustedProxies` empty when the
application is directly exposed — `X-Forwarded-For` is attacker-controlled, and
honouring it unconditionally would let anyone spoof their address past a rate
limit or an IP allowlist.

```go
kernel := http.NewKernel(app, http.KernelConfig{
    TrustedProxies: []string{"10.0.0.0/8"},
})
```

### Secure-by-default settings

These default to the safe value; the insecure setting is the one you opt into.

| Setting | Default | Notes |
|---|---|---|
| `APP_DEBUG` | `false` | Debug mode returns the underlying error and a stack trace to the client. |
| `session.secure` | `true` | Set to `false` only for local development over plain HTTP. |
| `session.http_only` | `true` | Keeps the session cookie away from JavaScript. |
| Local disk permissions | `0600` / `0700` | Override per-disk via `permissions` for a genuinely public disk. |

## CLI

```bash
# Install the CLI tool
go install github.com/genesysflow/go-genesys/cmd/genesys@latest

# Create a new project
genesys new myapp

# Generators
genesys make:controller UserController
genesys make:model User
genesys make:migration create_users_table
genesys make:middleware AuthMiddleware
genesys make:provider MyServiceProvider
genesys make:job SendEmail
genesys make:event OrderShipped
genesys make:listener SendReceipt
genesys make:seeder Users
genesys make:request StoreUser
genesys make:command SyncOrders
genesys make:policy Post

# Database
genesys migrate              # run pending migrations
genesys migrate:rollback     # rollback the last batch
genesys migrate:reset        # rollback everything
genesys migrate:fresh        # reset + re-run
genesys migrate:status
genesys db:seed [--seeder users]

# Runtime
genesys serve [--port=8080]
genesys queue:work [--queue=default --tries=3]
genesys queue:failed / queue:retry <id|all>
genesys schedule:work / schedule:run / schedule:list

# Utilities
genesys key:generate [--show]
genesys route:list
genesys about
```

## Configuration

Configuration files use YAML with environment interpolation:

```yaml
# config/app.yaml
name: MyApp
env: ${APP_ENV:local}
debug: ${APP_DEBUG:false}

# config/database.yaml
default: pgsql
connections:
  pgsql:
    driver: pgsql
    host: ${DB_HOST:localhost}
    database: ${DB_DATABASE:myapp}
```

```go
config := app.GetConfig()
name := config.GetString("app.name")
debug := config.GetBool("app.debug")
```

## Documentation

Per-topic guides live in [`docs/`](docs/):
routing, database, validation, cache, queue, auth, views, mail, scheduling, events, and more.

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request. CI runs `go vet` and the full test suite with the race detector.

## License

Go-Genesys is open-source software licensed under the [MIT license](LICENSE).

---

Built with ❤️ for the Go community
