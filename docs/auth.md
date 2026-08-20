# Authentication & Authorization

## User model

Any model can be authenticated by implementing two methods:

```go
type User struct {
    database.Model
    Email    string `db:"email" json:"email"`
    Password string `db:"password" json:"-"`
    ApiToken string `db:"api_token" json:"-"`
}

func (u *User) GetAuthIdentifier() any  { return u.ID }
func (u *User) GetAuthPassword() string { return u.Password }
```

## Wiring

```go
app.Register(&providers.AuthServiceProvider{
    UserProvider: auth.NewORMUserProvider[models.User](),
})
// Registers the "web" session guard and, because the ORM provider supports
// tokens, the "api" token guard.
```

The session guard requires the session middleware:

```go
kernel.UseFiber(sessionManager.Middleware())
```

## Logging in and out

```go
kernel.POST("/login", func(ctx *http.Context) error {
    user, err := authfacade.Attempt(ctx, map[string]any{
        "email":    ctx.Input("email"),
        "password": ctx.Input("password"),
    })
    if err != nil {
        return errors.Unauthorized("Invalid credentials")
    }
    return ctx.Resource(user)
})

kernel.POST("/logout", func(ctx *http.Context) error {
    return authfacade.Logout(ctx)
})
```

`Attempt` verifies the bcrypt hash (with a timing-safe miss path) and
regenerates the session ID on login to prevent fixation.

## Protecting routes

```go
guard := authfacade.Guard()          // default guard
api := authfacade.Guard("api")       // named guard

kernel.GET("/profile", Profile, auth.Middleware(guard))

// Browser apps: redirect instead of 401
auth.Middleware(guard, auth.MiddlewareOptions{RedirectTo: "/login"})

// Keep authenticated users away from /login
auth.GuestMiddleware(guard, "/dashboard")
```

Inside handlers:

```go
user := authfacade.User(ctx)   // nil when guest
id := authfacade.ID(ctx)
if authfacade.Check(ctx) { ... }
```

## Token guard

Clients send `Authorization: Bearer <token>`; the ORM provider matches it
against the `api_token` column (configurable via `TokenField`).

## Gates

```go
gate.Define("update-post", func(user auth.Authenticatable, args ...any) bool {
    post := args[0].(*models.Post)
    return post.AuthorID == user.GetAuthIdentifier()
})

gate.Before(func(user auth.Authenticatable, ability string, args ...any) *bool {
    if isAdmin(user) {
        yes := true
        return &yes // admins may do anything
    }
    return nil // fall through to the ability
})

if gate.Allows(user, "update-post", post) { ... }
if err := gate.Authorize(user, "update-post", post); err != nil {
    return err // 403
}
```

Generate policy scaffolds with `genesys make:policy Post`.
