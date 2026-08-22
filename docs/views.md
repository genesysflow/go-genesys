# Views

The view layer wraps Go's `html/template` with Laravel-style ergonomics.

## Setup

```go
app.Register(&providers.ViewServiceProvider{})
```

```yaml
# config/view.yaml
path: resources/views
reload: true            # re-parse on every render; defaults to true outside production
layout: layouts.app     # wraps every rendered page; omit for no layout
```

`path` is relative to the application root, not to the directory the
binary was started from.

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

## Layouts

A layout is an ordinary view that renders the page through `{{.content}}`.
Set it once and every page is wrapped in it - Laravel's `@extends`,
without a directive in every file.

```html
<!-- layouts/app.html -->
<html>
  {{template "partials.nav" .}}
  <main>{{.content}}</main>
</html>

<!-- users/index.html: just the page -->
<h1>Users</h1>
<ul>{{range .users}}<li>{{.Name}}</li>{{end}}</ul>
```

```yaml
# config/view.yaml
layout: layouts.app
```

The page is rendered first and passed to the layout as already-rendered
HTML, so its markup is not escaped on the way in. The layout receives the
page's data too, so `{{.title}}` works in both.

A page can name a different layout, or opt out of one entirely - a
fragment answering an ajax request has no business carrying the site's
chrome:

```go
ctx.ViewIn("layouts.print", "invoices.show", data)  // a different layout
ctx.ViewIn("", "partials.row", data)                // no layout
```

Rendering a layout by name renders it directly rather than wrapping it in
itself.

## Partials

Include one template from another with the standard `{{template}}`
action:

```html
{{template "partials.nav" .}}
```

A slot is data unless it says otherwise: a plain string is escaped, and
markup declares itself with `raw`.

```html
{{component "card" (dict "slot" .userComment)}}          <!-- escaped -->
{{component "card" (dict "slot" (raw "<p>markup</p>"))}} <!-- rendered -->
```

That matters because a slot is usually filled from a handler, and a
handler's data is a request's data.

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
