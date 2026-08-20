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
`lower`, `title`. All output is HTML-escaped by default.

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
