# Sessions

## Configuration

```yaml
# config/session.yaml
driver: file            # memory, file, database
files: storage/sessions
table: sessions         # database driver
cookie: myapp_session
secure: true            # false only for local plain-HTTP development
http_only: true
same_site: Lax
```

The database driver needs a table (`session.CreateSessionsTable` in a
migration) and resolves its connection lazily after the database provider
boots.

`files` is relative to the application root, not to the directory the
binary was started from.

## Middleware

The kernel installs the session middleware from the registered manager,
so registering the provider is all that is needed:

```go
app.Register(&providers.SessionServiceProvider{})
```

An application that places the middleware itself - a different store per
route group, say - turns the automatic one off:

```go
kernel := http.NewKernel(app, http.KernelConfig{DisableSession: true})
kernel.UseFiber(sessionManager.Middleware())
```

## Usage

```go
sess := session.GetFromContext(ctx.FiberCtx())

sess.Set("theme", "dark")
theme := sess.GetString("theme")
sess.Forget("theme")
sess.Flush()

sess.Regenerate() // rotate the session ID (done automatically on login)
```

## Flash data

Flash data lives for the rest of the current request and the next request
only:

```go
sess.Flash("status", "Profile updated!")

// Next request:
status := sess.GetString("status") // "Profile updated!"
// Request after that: gone.

sess.Reflash()          // keep all flash data one more request
sess.Keep("status")     // keep selected keys
```

## Old input

Repopulate forms after a validation redirect:

```go
sess.FlashInput(map[string]any{"email": ctx.Input("email")})
// After the redirect:
email := sess.Old("email")
if sess.HasOldInput() { ... }
```

## Redis Driver

```yaml
# config/session.yaml
driver: redis
redis:
  addr: ${REDIS_ADDR:-localhost:6379}
  prefix: "session:"
```

Sessions expire natively via Redis TTLs, and `Reset` only clears keys
under the configured prefix.
