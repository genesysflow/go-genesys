# Getting Started

## Installation

```bash
go install github.com/genesysflow/go-genesys/cmd/genesys@latest
genesys new myapp
cd myapp
go mod tidy
genesys key:generate
genesys serve
```

## Application lifecycle

A Go-Genesys application follows Laravel's lifecycle:

1. **Create** the application: `app := foundation.New()`
2. **Register** service providers: each provider's `Register` binds services into the container
3. **Boot**: `app.Boot()` runs every provider's `Boot` after all registrations
4. **Serve**: the HTTP kernel (or console kernel) takes over

```go
app := foundation.New()

app.Register(&providers.AppServiceProvider{})
app.Register(&providers.LogServiceProvider{})
app.Register(&providers.DatabaseServiceProvider{})
app.Register(&providers.CacheServiceProvider{})
app.Register(&providers.ViewServiceProvider{})

app.Boot()
```

## Project structure

```
myapp/
├── app/
│   ├── controllers/
│   ├── models/
│   ├── jobs/
│   ├── events/ listeners/ policies/ requests/
│   └── providers/
├── bootstrap/app.go        # application wiring
├── config/                 # YAML config with ${ENV:default} interpolation
├── database/
│   ├── migrations/
│   └── seeders/
├── lang/                   # translations (en/messages.yaml, ...)
├── resources/views/        # templates
├── routes/                 # web.go, api.go
├── storage/                # cache, logs, sessions
└── main.go
```

## Service container

```go
// Bind
app.Singleton("myservice", func() *MyService { return NewMyService() })
app.InstanceType(myService)              // bind by type

// Resolve
svc := container.MustResolve[*MyService](app)
svc, err := container.Resolve[*MyService](app)
```

## Writing a provider

```go
type BillingServiceProvider struct {
    providers.BaseProvider
}

func (p *BillingServiceProvider) Register(app contracts.Application) error {
    app.InstanceType(billing.NewGateway())
    return nil
}

func (p *BillingServiceProvider) Boot(app contracts.Application) error {
    return nil // runs after all providers are registered
}
```

## Validating Configuration at Boot

Fail fast on missing configuration instead of at first use:

```go
config.MustValidate(cfg,
    "app.key",
    "database.default",
    "mail.from_address",
)
```

All missing keys are reported together; empty strings count as missing.
