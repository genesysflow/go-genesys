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

A public page often still needs to know who is reading it - to greet
them, to show an edit link, to let a policy see a draft its author is
entitled to. `ResolveUser` puts the user on the context when there is
one and lets the request through when there is not:

```go
router.Use(auth.ResolveUser(guard))
```

Guarding a route is still `auth.Middleware`'s job. A user another
middleware already resolved is left alone, so this composes with a
test's acting user.

Inside handlers:

```go
user := authfacade.User(ctx)   // nil when guest
id := authfacade.ID(ctx)
if authfacade.Check(ctx) { ... }
```

## Retrieving the authenticated user

The guard stashes the user on the context, and there are four ways to
read it back - from the rawest to the one an application should use.

`ctx.User()` is the value itself, as `any`:

```go
user := ctx.User()          // any, nil for a guest
if ctx.HasUser() { ... }
```

`auth.UserFrom(ctx)` narrows that to `auth.Authenticatable`, nil for a
guest - enough when all you need is the identifier:

```go
if user := auth.UserFrom(ctx); user != nil {
    post.AuthorID = user.GetAuthIdentifier()
}
```

Do not type-assert the result. It panics on the request where it matters
most, the one from someone who is not signed in:

```go
user := auth.UserFrom(ctx).(*models.User)   // panics for a guest
```

`http.UserAs` is the typed accessor. It is two-value and never panics:

```go
user, ok := http.UserAs[models.User](ctx)
if !ok {
    return ctx.Unauthorized()
}
```

Wrap it once in your models package and handlers stop naming the type
parameter at all. This is the application's `$request->user()`:

```go
// app/models/current_user.go
package models

// CurrentUser returns the signed-in user, or nil for a guest.
func CurrentUser(ctx *http.Context) *User {
    user, _ := http.UserAs[User](ctx)
    return user
}

// AsUser narrows an auth.Authenticatable - what gates, policies and the
// password broker are handed - to *User. ok is false for a guest and for
// a user model this application does not own.
func AsUser(user auth.Authenticatable) (*User, bool) {
    account, ok := user.(*User)
    if !ok || account == nil {
        return nil, false
    }
    return account, true
}
```

Call sites then read like Laravel's:

```go
func (c *Controller) Drafts(ctx *http.Context) error {
    user := models.CurrentUser(ctx)
    // ...
}

func (p *PostPolicy) Publish(user auth.Authenticatable, post *models.Post) bool {
    editor, ok := models.AsUser(user)
    return ok && editor.IsEditor()
}
```

### Why not `ctx.User()` returning `*models.User` directly?

`http.Context` is defined in the framework and cannot name your
application's model. Go has no type parameters on methods, so the typed
accessor has to be a function rather than a method, and making `Context`
generic would put the user type in every handler, middleware and router
signature in the program. The narrowing therefore belongs to the
application - one line, in one file - which is what Laravel does
implicitly when `$request->user()` hands back your `App\Models\User`.

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

## Remember Me

`Remember`/`AttemptRemember` issue an HttpOnly `id|token` cookie backed
by a `remember_token` column (the ORM provider implements this
automatically). When the session is gone the guard logs the user back in
from the cookie; `Logout` expires the cookie and cycles the token so
stale cookies die everywhere:

```go
guard.AttemptRemember(ctx, map[string]any{
    "email":    email,
    "password": password,
})
```

## Password Reset

The broker issues single-use, hashed, throttled, expiring tokens over a
`password_reset_tokens (email, token, created_at)` table:

```go
broker := auth.NewPasswordBroker(conn.Driver(), conn, "")

token, err := broker.CreateToken(email)   // ErrResetThrottled when too soon
// mail the link containing token ...

err = broker.Consume(email, token, func() error {
    hashed, _ := hash.Make(newPassword)
    return users.UpdatePassword(email, hashed)
}) // token survives when the callback fails; deleted on success
```

The scaffolded login is rate-limited, keyed by address as well as IP so
one attacker cannot lock every account out and one office NAT is not one
bucket. Pass the application's cache store so the limit survives a
restart and holds across instances:

```go
controller := &auth.Controller{Guard: guard, Limiter: cacheStore}
```

`make:auth` scaffolds the whole flow. The framework does not know your
user model or how you send mail, so two hooks on the generated
controller do the parts that are yours - the same way `Create` handles
registration:

```go
controller := &auth.Controller{
    // ...
    Passwords: broker,
    SendLink: func(email, token string) error {
        return mail.SendDefault(&appmail.ResetLink{Email: email, URL: resetURL(email, token)})
    },
    UpdatePassword: func(user genesysauth.Authenticatable, hashed string) error {
        account := user.(*models.User)
        account.Password = hashed
        return database.Update(account)
    },
}
```

`UpdatePassword` runs inside `Consume`, so a token is spent only when
the password it was issued for was actually changed.

## Email Verification

Temporary signed links bind a user id to a hash of their email:

```go
verifier := auth.NewEmailVerifier([]byte(env.Require("APP_KEY")))
link, _ := verifier.VerificationURL("https://app.test/email/verify", user.ID, user.Email)

// In the verify handler:
id, err := verifier.Parse(fullRequestURL)     // signature + expiry
user := findUser(id)
err = verifier.Confirm(fullRequestURL, user.Email)
```

## Policies

A policy is a struct whose methods are the abilities. Method names map to
ability names by kebab-casing, so `ViewAny` answers `"view-any"`:

```go
type PostPolicy struct{}

func (p *PostPolicy) ViewAny(user auth.Authenticatable) bool { return user != nil }
func (p *PostPolicy) View(user auth.Authenticatable, post *models.Post) bool { return true }
func (p *PostPolicy) Create(user auth.Authenticatable) bool { return user != nil }
func (p *PostPolicy) Update(user auth.Authenticatable, post *models.Post) bool {
    return post.AuthorID == user.GetAuthIdentifier()
}

auth.RegisterPolicy[models.Post](gate, &PostPolicy{})
```

The gate then answers by model type:

```go
gate.Allows(user, "update", post)                  // -> PostPolicy.Update
auth.AllowsFor[models.Post](gate, user, "create")  // no instance to act on
```

As route middleware, `Can` loads the model from the route parameter by
primary key, and `CanBy` by any other column - a slug, a uuid, an email:

```go
router.PUT("/posts/:post", Update).
    Middleware(auth.Can[models.Post](gate, "update", "post"))

router.PUT("/posts/:post", Update).
    Middleware(auth.CanBy[models.Post](gate, "update", "post", "slug"))
```

A model that does not exist is a 404, decided before the policy is asked
about a model that is not there; a denied ability is a 403.

Decisions follow Laravel's order: before hooks, then an explicitly
defined ability, then the policy. Anything unanswered is denied.

Registration rejects a bool-returning method whose arguments are wrong -
that typo would otherwise deny silently for the life of the application.
Methods that do not return a bool are helpers and are left alone.
`auth.PolicyAbilities[models.Post](gate)` lists what resolved.

## Authorizing a request

`auth.GateMiddleware(gate)` binds the gate to every request, after
whatever sets the user:

```go
kernel.Use(auth.Middleware(guard), auth.GateMiddleware(gate))
```

Handlers and views then ask directly:

```go
func Update(ctx *http.Context) error {
    post, err := http.BindModel[models.Post](ctx, "post")
    if err != nil {
        return err
    }
    if err := ctx.Authorize("update", post); err != nil {
        return err // 403
    }
    ...
}
```

```html
{{if .gate.Allows "update" .post}}<a href="...">Edit</a>{{end}}
```

With no gate bound, `ctx.Can` denies: a forgotten middleware fails closed
rather than authorizing by accident.

Routes can be guarded directly, loading the model from the route
parameter (a missing one is a 404, a denial a 403):

```go
router.PUT("/posts/:post", UpdatePost).
    Middleware(auth.Can[models.Post](gate, "update", "post"))

router.POST("/posts", StorePost).
    Middleware(auth.CanAny[models.Post](gate, "create"))
```

## API tokens

Personal access tokens, in the shape Sanctum stores them. Create the
table from a migration:

```go
func (m *CreateTokensTable) Up(builder *schema.Builder) error {
    return auth.CreatePersonalAccessTokensTable(builder)
}
```

The `AuthServiceProvider` registers a `*auth.TokenRepository` (and a
`"sanctum"` guard) once a database connection exists:

```go
repository := container.MustResolve[*auth.TokenRepository](app)

plaintext, token, err := repository.Create(user, "cli", []string{"posts:read"}, nil)
// plaintext is the only time the token exists; the database holds a hash
```

Only the SHA-256 of the secret is stored and the comparison is
constant-time, so a leaked database hands over nothing usable and a wrong
token cannot be narrowed down by timing.

```go
guard := auth.NewPersonalAccessTokenGuard("api", repository, provider)

kernel.GET("/api/posts", ListPosts,
    auth.Middleware(guard),
    auth.RequireAbility("posts:read"))
```

Inside a handler, `auth.TokenFrom(ctx)` returns the token that
authenticated the request, with `Can`/`Cannot` for its abilities. Other
repository operations: `Revoke`, `RevokeAll` (sign out everywhere),
`ListFor`, and `PruneExpired`.

A token records its owner polymorphically. `tokenable_type` holds the
model's table name, the same value every other polymorphic column in the
framework stores, so moving the model to another package does not orphan
the tokens already issued.

An API authenticated by bearer token needs no CSRF protection - without
a cookie there is no cross-site request to forge - so exempt it:

```go
middleware.CSRF(middleware.CSRFConfig{
    Secret: secret,
    Except: []string{"/api/"},
})
```

The exemption is a path prefix, and an exempt path still gets a token
issued, so a form rendered by one can post to a guarded path.
