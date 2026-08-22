# Routing

## Basics

```go
router.GET("/", HomeController)
router.POST("/users", StoreUser)
router.PUT("/users/:id", UpdateUser)
router.Any("/ping", Pong)
router.Match([]string{"GET", "POST"}, "/either", Handler)
```

Route parameters use `:name` and are read with `ctx.Param("name")` /
`ctx.ParamInt("name")`.

## Named routes and URL generation

```go
router.GET("/users/:id", ShowUser).Name("users.show")

url := router.URL("users.show", map[string]any{"id": 42}) // /users/42

// or via the facade
route.URL("users.show", map[string]any{"id": 42})
```

## Groups

```go
router.Group("/admin", func(r *http.Router) {
    r.GET("/dashboard", Dashboard)
    r.Group("/reports", func(rr *http.Router) {
        rr.GET("/", ListReports) // /admin/reports
    })
}, adminMiddleware)
```

## Middleware groups and aliases

```go
kernel.DefineMiddlewareGroup("web", sessionMiddleware, csrfMiddleware)
kernel.AliasMiddleware("auth", auth.Middleware(guard))

kernel.Group("/app", routes, kernel.MiddlewareGroup("web")...)
kernel.GET("/profile", Profile, kernel.Named("auth")...)
```

## Redirects and fallback

```go
router.Redirect("/old", "/new")        // 302
router.Redirect("/legacy", "/new", 301)

// After registering all routes:
router.Fallback(func(ctx *http.Context) error {
    return ctx.Status(404).JSONResponse(map[string]any{"message": "not found"})
})
```

## Route model binding

```go
router.GET("/users/:user", func(ctx *http.Context) error {
    user, err := http.BindModel[models.User](ctx, "user") // 404 when missing
    if err != nil {
        return err
    }
    return ctx.Resource(user)
})

// Bind by a custom column
post, err := http.BindModelBy[models.Post](ctx, "slug", "slug")
```

## JSON responses

```go
ctx.Resource(user)                    // {"data": {...}}
ctx.ResourceWith(users, meta)         // {"data": [...], "meta": {...}}
ctx.Paginated(page)                   // splits a paginator into data and meta
ctx.CreatedResource(user)             // 201 with the same data envelope
ctx.Created(payload)                  // 201, unwrapped
```

`CreatedResource` is the one to reach for after writing a record, so a
client reads `data.*` whether it just created the resource or fetched it.
`Created` sends the body as-is, for a payload that is not a resource -
the plaintext of a freshly issued token, say.

## Redirects that came from the request

`RedirectTo` names a target you chose, so an external one is fine. A
target the request supplied is not:

```go
ctx.Intended("/dashboard")   // where a guest was headed, if it is on this host
ctx.Back("/posts")           // the Referer, if it is on this host
```

The auth middleware records the destination when it sends a guest to the
login page, so `Intended` is what a login handler returns. Writing
`ctx.RedirectTo(ctx.Query("next"))` instead is an open redirect.

## Resource controllers

```go
router.Resource("photos", photoController)      // index/create/store/show/edit/update/destroy
router.APIResource("videos", videoController)   // no create/edit
```

## Subdomain Routing

`Domain` groups routes under a host pattern; `{name}` segments capture
into domain parameters, and non-matching hosts fall through to later
routes on the same path:

```go
router.Domain("{account}.example.com", func(r *http.Router) {
    r.GET("/dashboard", func(ctx *http.Context) error {
        account := http.DomainParam(ctx, "account")
        return ctx.String("Hello, " + account)
    })
})

router.GET("/dashboard", apexDashboard) // example.com keeps working
```

## Request helpers

Inside a handler the context carries the request's identity, so nothing
has to be re-resolved:

```go
func Show(ctx *http.Context) error {
    user, ok := http.UserAs[models.User](ctx)  // set by auth.Middleware
    sess := ctx.Session()                      // nil without session middleware
    email := ctx.Old("email")                  // input flashed last request

    if ctx.RouteIs("users.*") {
        // ctx.Route() / ctx.RouteName() describe the matched route
    }
    _ = user
    _ = ok
    _ = sess
    _ = email
    return nil
}
```

## Redirects

```go
ctx.RedirectTo("/dashboard").Send()
ctx.RedirectToRoute("users.show", map[string]any{"user": 42}).Send()

ctx.Back("/users/create").
    WithErrors(err).      // map[string][]string, *errors.ValidationError, or any error
    WithInput().          // repopulates the form; password fields are dropped
    With("status", "Saved!").
    Status(303).
    Send()
```

`Back` honours the `Referer` only when it points at this host, so a
handler redirecting back is never an open redirect. Flashed values are
read back with `ctx.Errors()`, `ctx.Old()`, and the session, and are
shared with views automatically.
