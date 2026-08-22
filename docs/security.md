# Security

What the framework defends against on your behalf, what it hands you to
defend with, and the places where the safe thing has to be the thing you
type.

Every claim here is covered by a test; the adversarial ones live in
`example/tests/security_test.go`, which drives them through the real
routes.

## What is handled for you

| | |
|---|---|
| SQL injection | Values are bound, never interpolated. Identifiers are quoted, operators come from an allowlist, and an ORDER BY direction is normalised to ASC/DESC. |
| XSS | Templates escape by default, including a page rendered into a layout and a value passed to a component. |
| CSRF | Signed double-submit tokens, with the Origin (or Referer) checked against the host. |
| Session fixation | The session id is rotated on login and on logout. |
| Open redirect | `Back` and `Intended` honour a destination only when it stays on this host. |
| Response splitting | Header values cannot terminate a header. |
| Timing attacks | Tokens and CSRF values are compared in constant time. |
| Credential storage | Passwords are bcrypt; API and reset tokens are stored as SHA-256 of a high-entropy secret. |
| Account enumeration | Login and password-reset answer the same way whether or not the address is registered. |
| Error disclosure | Debug is off unless asked for; without it, a 500 says nothing about SQL, paths or versions. |

## The places you have to type the safe thing

These are the escape hatches. Each one exists because it is sometimes
necessary; none of them should ever receive client input.

```go
builder.SelectRaw("COUNT(*) AS total")   // SQL, not data
builder.WhereRaw("...", bindings...)     // bindings are safe; the SQL is not
builder.OrderByRaw("...")
{{raw .html}}                            // trusted markup, unescaped
```

`Select`, `Where`, `OrderBy` and the template's `{{.value}}` are the safe
forms, and they are what an application should use for anything a client
sends. A column list from a query string goes to `Select`, which quotes
it; a value goes to `Where`, which binds it.

## Requests

**Bind only what you declared.** A form request binds the fields its
struct names, which is what keeps `role`, `author_id` and `id` out of
reach:

```go
type StorePostRequest struct {
    Title string `json:"title" validate:"required,min=3"`
    Body  string `json:"body"  validate:"required,min=20"`
}
```

Ownership comes from the session, never from the payload:

```go
post := &models.Post{AuthorID: auth.UserFrom(ctx).GetAuthIdentifier(), Title: req.Title}
```

`ctx.Bind` fills a struct wholesale. It clears a model's primary key and
timestamps afterwards, so it cannot be used to choose which row you are
writing - but it will still fill every other column the request names.
Prefer a form request for anything a client sends.

## Redirects

`RedirectTo` names a target you chose, so an external one is legitimate.
A target that came from the request is different:

```go
ctx.Intended("/dashboard")   // where the guest was headed, if it is on this host
ctx.Back("/posts")           // the Referer, if it is on this host
```

Never write `ctx.RedirectTo(ctx.Query("next"))`. That is the open
redirect `Intended` exists to replace; the auth middleware records the
destination for you.

## Authentication

`make:auth` scaffolds a flow that rotates the session on login, rate
limits the endpoints that check a credential, answers identically for
known and unknown addresses, and never flashes a password back into a
form. Give it the application's cache store so the rate limit holds
across restarts and instances:

```go
controller := &auth.Controller{Guard: guard, Limiter: cacheStore}
```

Authorize every write with the policy that governs it, on the route, so
the handler never runs for a caller who may not:

```go
router.PUT("/posts/:post", Update).
    Middleware(auth.CanBy[models.Post](gate, "update", "post", "slug"))
```

A missing model is a 404, decided before the policy is asked; a denied
ability is a 403. With no gate bound, `ctx.Can` denies.

## Configuration that decides whether you are safe

| Setting | Safe value | What goes wrong |
|---|---|---|
| `app.key` | 32 bytes, generated | Without it CSRF protection stays off |
| `app.debug` | `false` | On, a 500 hands the client SQL, paths and versions |
| `session.secure` | `true` | Off, the session cookie travels in clear and is replayable |
| `session.http_only` | `true` | Off, a script can read the session |
| `cors.allow_origins` | an explicit list | `*` with credentials is a same-origin-policy bypass, and panics |

The framework defaults all of these safely and warns when a production
application hands out a cookie that is not Secure. An application that
overrides a default for local development should do it in `.env`, which
is not deployed - not in `config/`, which is.

Add the headers that cost nothing:

```go
middleware.Secure()   // nosniff, SAMEORIGIN, strict-origin-when-cross-origin
```

HSTS and a Content-Security-Policy are left to you: HSTS locks clients
out of a host that is not fully served over TLS, and an effective CSP is
application-specific. Set both once you know they fit.

## Things the framework does not decide for you

- **SSRF.** A webhook notification posts to the URL its notifiable
  routes to. If your application lets users register that URL, validate
  it against your own allowlist before storing it.
- **File uploads.** Content type and size are yours to check.
- **Authorization logic.** Policies are enforced wherever you attach
  them; a route with no `Can` is a route with no check.
- **Secrets.** `.env` is gitignored by default. Keep it that way.
