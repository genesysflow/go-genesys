# Go-Genesys

A Laravel-inspired web framework for Go, providing elegant syntax and powerful features for building modern web applications.

## Features

- **Service Container**: Dependency injection powered by [samber/do](https://github.com/samber/do)
- **Service Providers**: Register and bootstrap application services
- **HTTP Layer**: Built on [Fiber](https://github.com/gofiber/fiber) for blazing fast HTTP handling
- **Middleware Pipeline**: Before/after middleware with Laravel-style syntax
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

## CLI Tool

Go-Genesys includes a CLI tool for scaffolding and development:

```bash
# Create a new project
genesys new myapp

# Generate a service provider
genesys make:provider MyServiceProvider

# Generate a controller
genesys make:controller UserController

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

