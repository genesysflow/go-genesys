# Go-Genesys

A Laravel-inspired web framework for Go, providing elegant syntax and powerful features for building modern web applications.

## Features

- **Service Container**: Dependency injection container for managing application services
- **Service Providers**: Register and bootstrap application services with a clean lifecycle
- **HTTP Layer**: Built on [Fiber](https://github.com/gofiber/fiber) for blazing fast HTTP handling
- **Middleware Pipeline**: Powerful middleware system with before/after hooks
- **Database ORM**: Eloquent-style ORM with model support for easy database interaction
- **Query Builder**: Fluent SQL query builder with grammar abstraction
- **Schema Builder**: Define database schemas programmatically with migrations
- **Migrations**: Database schema version control and migration management
- **Configuration**: YAML-based config files with dot-notation access
- **Environment**: `.env` file support with type-safe helpers
- **Validation**: Struct-based validation with custom rules and error handling
- **Sessions**: Multiple session drivers (memory, file, database, redis)
- **Cache**: Flexible caching layer with multiple drivers (memory, redis, file)
- **Queue**: Background job processing with sync and async drivers
- **Events**: Event dispatcher for decoupled application components
- **Filesystem**: Unified filesystem abstraction (local, S3, and more)
- **Logging**: Structured logging with multiple channels and formatters
- **Error Handling**: Graceful panic recovery and detailed error reporting
- **Console Kernel**: CLI application framework with custom commands
- **Testing Helpers**: Built-in testing utilities for HTTP and database testing
- **Facades**: Static-like accessors for core services (DB, Storage, etc.)

## Installation

```bash
go get github.com/genesysflow/go-genesys
```

## Quick Start

```go
package main

import (
    "github.com/genesysflow/go-genesys/foundation"
    "github.com/genesysflow/go-genesys/container"
    "github.com/genesysflow/go-genesys/http"
    "github.com/genesysflow/go-genesys/providers"
)

func main() {
    // Create a new application instance
    app := foundation.New()
    
    // Register service providers
    app.Register(&providers.AppServiceProvider{})
    app.Register(&providers.RouteServiceProvider{})
    
    // Bootstrap the application
    app.Boot()
    
    // Get the HTTP kernel and run the server
    kernel := container.MustResolve[*http.Kernel](app, "http.kernel")
    kernel.Run(":3000")
}
```

## Application Lifecycle

Inspired by Laravel, Go-Genesys follows a well-defined lifecycle:

### Bootstrap Phase (once at startup)

1. **Application Creation**: Create new application instance with container
2. **Provider Registration**: Register all service providers (binds services to container)
3. **Application Boot**: Boot all providers, load configuration
4. **Server Start**: HTTP kernel starts listening for requests

### Request Phase (per request)

1. **Request Entry**: HTTP request enters the application
2. **Middleware Stack**: Request passes through global and route middleware
3. **Routing**: Router dispatches request to the appropriate handler
4. **Controller**: Business logic is executed
5. **Response**: Response travels back through middleware and is sent to the client

## Database & Migrations

Go-Genesys provides a powerful database abstraction layer.

### Migrations

Define your database schema using Go code:

```go
func (m *CreateUsersTable) Up(builder *schema.Builder) error {
    return builder.Create("users", func(table *schema.Blueprint) {
        table.ID()
        table.String("name", 255)
        table.String("email", 255).Unique()
        table.Timestamps()
    })
}
```

### Models

Define your models by embedding `database.Model`:

```go
type User struct {
    database.Model
    Name  string `json:"name" db:"name"`
    Email string `json:"email" db:"email"`
}
```

Retrieve records:

```go
// Get all users
users, _ := database.All[User]()

// Find by ID
user, _ := database.Find[User](1)

// Create a new record
user := &User{Name: "John", Email: "john@example.com"}
id, _ := database.Create(user)

// Update a record
database.Update(1, user)

// Delete a record
database.Delete[User](1)
```

### Query Builder

Fluent interface for building queries:

```go
// Get all users as maps
results, _ := db.Table("users").Get()

// Complex queries
db.Table("users").
    Where("age", ">", 18).
    OrderBy("created_at", "desc").
    Limit(10).
    Get()

// Joins
db.Table("users").
    Join("posts", "users.id", "=", "posts.user_id").
    Select("users.name", "posts.title").
    Get()

// Aggregates
count, _ := db.Table("users").Count()
avg, _ := db.Table("orders").Avg("total")
```

## Additional Features

### Cache

Multiple cache drivers for flexible caching strategies:

```go
// Get cache store
store, _ := cacheManager.Store()

// Store data in cache
store.Put("key", value, 60*time.Minute)

// Retrieve from cache
value, err := store.Get("key")

// Forget a key
store.Forget("key")

// Flush all cache
store.Flush()
```

### Queue

Process background jobs asynchronously:

```go
// Define a job
type SendEmailJob struct {
    Email   string
    Message string
}

func (j *SendEmailJob) Handle() error {
    return sendEmail(j.Email, j.Message)
}

// Get queue connection and push a job
queue, _ := queueManager.Connection()
queue.Push(&SendEmailJob{
    Email:   "user@example.com",
    Message: "Welcome!",
})
```

### Events

Decouple application components with events:

```go
// Define an event
type UserRegistered struct {
    User *User
}

func (e *UserRegistered) Name() string {
    return "user.registered"
}

// Define a listener
func SendWelcomeEmail(event events.Event) error {
    // Type assert to get the specific event
    userEvent := event.(*UserRegistered)
    // Send welcome email using userEvent.User
    return nil
}

// Create dispatcher and register listener
dispatcher := events.NewDispatcher()
dispatcher.Listen("user.registered", SendWelcomeEmail)

// Dispatch event
dispatcher.Dispatch(&UserRegistered{User: user})
```

### Filesystem

Unified interface for file operations across different storage systems:

```go
// Get filesystem disk (default or named)
disk := filesystemManager.Disk()

// Store files
disk.Put("file.txt", []byte("content"))

// S3 storage
s3Disk := filesystemManager.Disk("s3")
s3Disk.Put("bucket/file.txt", []byte("content"))

// Read files
content, _ := disk.Get("file.txt")

// Check existence
exists := disk.Exists("file.txt")

// Delete files
disk.Delete("file.txt")
```

### Validation

Powerful struct-based validation:

```go
type CreateUserRequest struct {
    Name  string `json:"name" validate:"required,min=3,max=255"`
    Email string `json:"email" validate:"required,email"`
    Age   int    `json:"age" validate:"required,min=18,max=100"`
}

// Validate
validator := validation.New()
result := validator.Validate(request)
if result.Fails() {
    // Handle validation errors
    errors := result.Errors()
    firstError := result.First()
}
```

## CLI Tool

Go-Genesys includes a powerful CLI tool for scaffolding and development:

```bash
# Install the CLI tool
go install github.com/genesysflow/go-genesys/cmd/genesys@latest

# Create a new project
genesys new myapp

# Generate components
genesys make:provider MyServiceProvider    # Generate a service provider
genesys make:controller UserController     # Generate a controller
genesys make:model User                    # Generate a model
genesys make:middleware AuthMiddleware     # Generate middleware
genesys make:migration create_users_table  # Generate a migration

# Database migrations
genesys migrate                  # Run pending migrations
genesys migrate:rollback         # Rollback the last migration batch
genesys migrate:status           # Check migration status
genesys migrate:fresh            # Drop all tables and re-run migrations
genesys migrate:reset            # Rollback all migrations

# Development
genesys serve                    # Start the development server
genesys serve --port=8080        # Start server on custom port
```

## Architecture

### Service Container

The service container is the core of Go-Genesys, managing dependency injection and service resolution:

```go
// Bind a service to the container (transient)
app.Bind("myservice", func() *MyService {
    return &MyServiceImpl{}
})

// Bind as singleton
app.Singleton("myservice", func() *MyService {
    return &MyServiceImpl{}
})

// Resolve a service
service, _ := app.Make("myservice")
```

### Service Providers

Service providers are the central place to register and bootstrap application services:

```go
type MyServiceProvider struct {
    providers.Provider
}

func (p *MyServiceProvider) Register(app contracts.Application) {
    // Bind services to the container
    app.Bind(func() *MyService {
        return NewMyService()
    })
}

func (p *MyServiceProvider) Boot(app contracts.Application) {
    // Bootstrap services after all providers are registered
}
```

### HTTP Kernel

The HTTP kernel handles the request lifecycle and middleware pipeline:

```go
kernel := http.NewKernel(app)

// Add global middleware
kernel.Use(middleware.Logger())
kernel.Use(middleware.Recovery())

// Start the server
kernel.Run(":3000")
```

### Routing

Define routes with a familiar, expressive syntax:

```go
// routes/web.go
func RegisterWebRoutes(router contracts.Router) {
    router.Get("/", controllers.HomeController)
    
    router.Group("/users", func(r contracts.Router) {
        r.Get("/", controllers.GetUsers)
        r.Get("/:id", controllers.GetUser)
        r.Post("/", controllers.CreateUser)
        r.Put("/:id", controllers.UpdateUser)
        r.Delete("/:id", controllers.DeleteUser)
    })
    
    // With middleware
    router.Group("/admin", func(r contracts.Router) {
        r.Get("/dashboard", controllers.AdminDashboard)
    }).Middleware(middleware.Auth())
}
```

## Project Structure

A typical Go-Genesys application follows this structure:

```
myapp/
├── app/
│   ├── controllers/     # HTTP controllers
│   ├── middleware/      # Custom middleware
│   ├── models/          # Database models
│   ├── providers/       # Service providers
│   └── services/        # Business logic services
├── bootstrap/
│   └── app.go           # Application bootstrap
├── config/              # Configuration files (YAML)
│   ├── app.yaml
│   ├── database.yaml
│   ├── filesystem.yaml
│   ├── logging.yaml
│   └── session.yaml
├── database/
│   └── migrations/      # Database migrations
├── routes/              # Route definitions
│   ├── api.go
│   ├── web.go
│   └── routes.go
├── storage/             # Application storage
│   ├── cache/
│   ├── logs/
│   └── sessions/
├── .env                 # Environment variables
├── go.mod
└── main.go              # Application entry point
```

## Testing

Go-Genesys provides testing utilities for HTTP and database testing:

```go
func TestUserController(t *testing.T) {
    // Create test request
    req := http.Get("/users")
    
    // Make request (would be executed against test server)
    // Note: Full integration testing requires additional setup
    resp := req.WithHeader("Accept", "application/json")
    
    // The testing package provides utilities for making HTTP requests
    // against your application during tests
}
```

## Configuration

Configuration files use YAML format and support environment-specific overrides:

```yaml
# config/app.yaml
name: MyApp
env: ${APP_ENV:local}
debug: ${APP_DEBUG:true}
url: ${APP_URL:http://localhost:3000}

# config/database.yaml
default: mysql
connections:
  mysql:
    driver: mysql
    host: ${DB_HOST:localhost}
    port: ${DB_PORT:3306}
    database: ${DB_DATABASE:myapp}
    username: ${DB_USERNAME:root}
    password: ${DB_PASSWORD:}
```

Access configuration values:

```go
// Get config instance (typically from service container)
config := app.Config()

// Get config value
appName := config.Get("app.name")

// Get as specific types
env := config.GetString("app.env")
debug := config.GetBool("app.debug")
port := config.GetInt("app.port")
```

## Documentation

For detailed documentation, visit the [documentation site](https://github.com/genesysflow/go-genesys).

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

Go-Genesys is open-source software licensed under the [MIT license](LICENSE).
---

Built with ❤️ for the Go community

