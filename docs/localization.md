# Localization

## Translation files

One folder per locale; YAML (or JSON) files become key prefixes:

```
lang/
├── en/
│   ├── messages.yaml    # welcome: "Welcome, :name!"
│   └── validation.yaml
└── es/
    └── messages.yaml    # welcome: "¡Bienvenido, :name!"
```

Nested keys flatten with dots: `messages.nested.deep`.

## Configuration

```yaml
# config/app.yaml
locale: en
fallback_locale: en

# config/lang.yaml (optional)
path: lang
```

```go
app.Register(&providers.LangServiceProvider{})
```

## Usage

```go
import langfacade "github.com/genesysflow/go-genesys/facades/lang"

langfacade.Trans("messages.welcome", map[string]string{"name": "Ana"})
// active locale first, then the fallback; missing keys return the key itself

langfacade.SetLocale("es")
langfacade.Locale() // "es"
```

## Pluralization

```yaml
# lang/en/messages.yaml
apples: "one apple|:count apples"
```

```go
langfacade.TransChoice("messages.apples", 1)  // "one apple"
langfacade.TransChoice("messages.apples", 5)  // "5 apples"
```

`:count` is replaced automatically; other `:placeholder` values come from
the optional replacements map.
