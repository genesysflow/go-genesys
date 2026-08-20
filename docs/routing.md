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
