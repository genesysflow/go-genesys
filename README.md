# Go-Genesys

A Laravel-inspired web framework for Go, providing elegant syntax and powerful features for building modern web applications.

## Features

- **Service Container**: Dependency injection powered by [samber/do](https://github.com/samber/do)
- **Service Providers**: Register and bootstrap application services
- **HTTP Layer**: Built on [Fiber](https://github.com/gofiber/fiber) for blazing fast HTTP handling
- **Middleware Pipeline**: Before/after middleware with Laravel-style syntax
- **Database ORM**: Eloquent-style ORM for easy database interaction
- **Query Builder**: Fluent SQL query builder
- **Migrations**: Database schema version control
- **Configuration**: YAML/JSON config files with dot-notation access
- **Environment**: `.env` file support with type-safe helpers
- **Validation**: Struct-based validation with custom rules
- **Sessions**: Multiple session drivers (memory, file, redis)
- **Logging**: Structured logging with multiple channels
- **Error Handling**: Graceful panic recovery and error reporting
- **CLI Tool**: Code generation and development commands

## Installation

```bash
go get github.com/genesysflow/go-genesys
```

## Quick Start

```go
package main

import (
    "github.com/genesysflow/go-genesys/foundation"
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
    kernel := foundation.MustMake[*http.Kernel](app)
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
users, _ := database.All[User]("users")

// Find by ID
user, _ := database.Find[User]("users", 1)
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
```

## CLI Tool

Go-Genesys includes a CLI tool for scaffolding and development:

```bash
# Install the tool
go install github.com/genesysflow/go-genesys/cmd/genesys@latest

# Create a new project
genesys new myapp

# Generate a service provider
genesys make:provider MyServiceProvider

# Generate a controller
genesys make:controller UserController

# Generate a model
genesys make:model User

# Generate a migration
genesys make:migration create_users_table

# Run migrations
genesys migrate

# Rollback migrations
genesys migrate:rollback

# Check migration status
genesys migrate:status

# Generate middleware
genesys make:middleware AuthMiddleware

# Run the development server
genesys serve
```

## Project Structure

A typical Go-Genesys application follows this structure:

```
myapp/
├── app/
│   ├── controllers/     # HTTP controllers
│   ├── middleware/      # Custom middleware
│   └── providers/       # Service providers
├── config/              # Configuration files
├── database/            # Migrations and seeds
├── routes/              # Route definitions
├── storage/             # Logs, cache, sessions
├── .env                 # Environment variables
├── go.mod
└── main.go
```

## Documentation

For detailed documentation, visit the [documentation site](https://github.com/genesysflow/go-genesys).

## License

Go-Genesys is open-source software licensed under the [MIT license](LICENSE).

