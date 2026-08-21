# Views

The view layer wraps Go's `html/template` with Laravel-style ergonomics.

## Setup

```go
app.Register(&providers.ViewServiceProvider{})
```

```yaml
# config/view.yaml
path: resources/views
reload: true   # re-parse on every render; defaults to true outside production
```

## Naming and rendering

Every file under the views directory is addressable by dot notation:

```
resources/views/
├── layouts/app.html      -> "layouts.app"
├── partials/nav.html     -> "partials.nav"
└── users/index.html      -> "users.index"
```

```go
func Index(ctx *http.Context) error {
    return ctx.View("users.index", map[string]any{
        "users": users,
        "title": "Users",
    })
}

// Outside a request
html, err := view.Render("emails.welcome", data)
```

## Layouts and partials

Compose templates with the standard `{{template}}` action:

```html
<!-- users/index.html -->
{{template "layouts.app" .}}

<!-- layouts/app.html -->
<html>
  {{template "partials.nav" .}}
  <main>{{.content}}</main>
</html>
```

## Shared data and helpers

```go
viewManager.Share("appName", "MyApp")         // available in every template
viewManager.AddFunc("money", formatMoney)     // custom template function
```

Built-in helpers: `raw` (opt out of escaping for trusted HTML), `upper`,
`lower`, `title`, `dict`. All output is HTML-escaped by default.

## Request helpers

Every template also gets these, wired by the service providers:

```html
<a href="{{route "users.show" (dict "user" .user.ID)}}">Profile</a>
<link rel="stylesheet" href="{{asset "css/app.css"}}">
<a href="{{url "/about"}}">{{trans "nav.about"}}</a>
<title>{{config "app.name"}}</title>

<form method="POST" action="{{route "users.update" (dict "user" 1)}}">
  {{csrf_field .csrf_token}}
  {{method_field "PUT"}}
</form>
```

`url` and `asset` build on `app.url`; `route` resolves named routes;
`trans` and `config` reach the translator and configuration. Unwired
helpers render empty rather than failing to parse.

## Per-request data

`ctx.View` shares these under whatever the handler passes, so a form
never has to thread them through:

| Key | What it holds |
| --- | --- |
| `errors` | the message bag flashed by the last validation failure |
| `old` | input flashed by the previous request |
| `session` | the request session |
| `user` | the authenticated user, or nil |
| `csrf_token` | the CSRF token, when the middleware ran |

```html
{{if .errors.Has "email"}}<span>{{.errors.First "email"}}</span>{{end}}
<input name="email" value="{{.old.Get "email"}}">
{{if .user}}Signed in as {{.user.Email}}{{end}}
```

Handler-supplied keys win, so passing your own `errors` overrides the bag.

## Composers

A composer fills in data for every view matching a pattern, so a partial
can carry its own data instead of every handler remembering to pass it:

```go
viewManager.Composer("partials.*", func(data map[string]any) {
    data["unreadCount"] = unread()
})
```

Composers run in registration order, before the handler's data is layered
on top - they supply defaults and never override what the handler passed.
`*` matches every view.

## Components

Templates under `components/` are reusable partials, called with
`component` and an inline `dict`; slot content passes as the `slot` key
and renders with `{{slot .}}`:

```html
<!-- components/alert.html -->
<div class="alert alert-{{.type}}">{{.message}}</div>

<!-- components/card.html -->
<div class="card"><h2>{{.title}}</h2>{{slot .}}</div>

<!-- any view -->
{{component "alert" (dict "type" "error" "message" .err)}}
{{component "card" (dict "title" "Hello" "slot" "<p>Body</p>")}}
```

Regular data stays HTML-escaped inside components; only slot content is
injected as-is.
