# Validation & Form Requests

## Form requests

Define a struct with `validate` tags and bind it in one call:

```go
type StoreUserRequest struct {
    Name  string `json:"name" validate:"required,min=3,max=100"`
    Email string `json:"email" validate:"required,email"`
    Age   int    `json:"age" validate:"omitempty,min=18"`
}

func StoreUser(ctx *http.Context) error {
    req, err := http.ValidateRequest[StoreUserRequest](ctx)
    if err != nil {
        return err
    }
    // req is fully bound and valid here
}
```

Binding follows the request: JSON and form bodies for POST/PUT/PATCH, the
query string for GET/HEAD/DELETE (tag fields with `query:"name"` too for
query binding).

A failed validation returns an `*errors.ValidationError`. How it is
rendered depends on the client. API clients get Laravel's 422 shape:

```json
{
  "message": "The given data was invalid.",
  "errors": {
    "name": ["Name must be at least 3 characters"],
    "email": ["Email must be a valid email address"]
  }
}
```

A browser posting a form is redirected back to it instead, with the
messages and its input flashed to the session - so the form repopulates
and shows its errors, exactly as Laravel does:

```go
// The handler is unchanged: `return err` does the right thing for both.
req, err := http.ValidateRequest[StoreUserRequest](ctx)
if err != nil {
    return err
}
```

```html
{{if .errors.Has "email"}}<span>{{.errors.First "email"}}</span>{{end}}
<input name="email" value="{{.old.Get "email"}}">
```

The redirect needs a session to flash into; without session middleware
the 422 shape is used for browsers too, rather than a redirect that
silently loses the errors. Password fields are never flashed.

To redirect by hand:

```go
return ctx.Back("/users/create").
    WithErrors(err).
    WithInput().
    With("status", "Please fix the errors below.").
    Send()
```

## Form request lifecycle

A request type may implement any of these to take part in the lifecycle.
All are optional:

```go
type StorePostRequest struct {
    Title string `json:"title" validate:"required"`
    Slug  string `json:"slug"  validate:"required"`
    Kind  string `json:"kind"  validate:"required"`
}

// 1. Normalise the payload before the rules run.
func (r *StorePostRequest) PrepareForValidation(ctx *http.Context) error {
    if r.Slug == "" {
        r.Slug = support.Str.Slug(r.Title)
    }
    return nil
}

// 2. Decide whether the caller may make this request at all (403 when
//    false). Checked before validation, so an unauthorized request never
//    learns which fields exist.
func (r *StorePostRequest) Authorize(ctx *http.Context) bool {
    return auth.UserFrom(ctx) != nil
}

// 3. Rules that tags cannot express, because they depend on the payload.
//    They run against the prepared request, so the slug derived above is
//    checked rather than skipped.
func (r *StorePostRequest) Rules() map[string]string {
    if r.Kind == "company" {
        return map[string]string{"vat": "required"}
    }
    return nil
}

// 4. Message and attribute overrides, scoped to this request only.
func (r *StorePostRequest) Messages() map[string]string {
    return map[string]string{"title.required": "Give the post a title."}
}

func (r *StorePostRequest) Attributes() map[string]string {
    return map[string]string{"vat": "VAT number"}
}

// 5. Cross-field checks, run once the rules pass.
func (r *StorePostRequest) AfterValidation(ctx *http.Context) map[string][]string {
    if r.Slug == r.Title {
        return map[string][]string{"slug": {"The slug must differ from the title."}}
    }
    return nil
}
```

## Database rules

`unique` and `exists` query the database. They are wired automatically by
the `ValidationServiceProvider` when a connection is configured:

```go
type SignupRequest struct {
    Email  string `json:"email"  validate:"required,email,unique=users.email"`
    TeamID int64  `json:"team_id" validate:"required,exists=teams.id"`
}
```

Updates ignore the record's own row, Laravel's third argument:

```go
func (r *UpdateUserRequest) Rules() map[string]string {
    return map[string]string{
        "email": fmt.Sprintf("unique=users.email.%d", r.ID),   // ignores id = r.ID
        // "unique=users.email.7.team_id" names the column to ignore on
    }
}
```

Both rules **fail closed**: with no database available the check cannot be
answered, and passing would wave through the duplicate the rule exists to
stop. Table and column names must be plain identifiers.

`unique` doubles as go-playground's slice-distinctness rule; a param that
is not `table.column` keeps that meaning (`validate:"unique"`).

## Attributes that were not submitted

A rule describes a value, so an attribute the request did not carry has
nothing for `max`, `email` or `unique` to describe, and those rules are
skipped for it - as they are in Laravel. Rules about presence itself
(`required`, `required_if`, `prohibited`, ...) still apply, which is how
a missing field is caught:

```go
v.ValidateMap(map[string]any{}, map[string]string{
    "slug":  "max=140",          // skipped: no slug was submitted
    "email": "required,email",   // fails on required, not on email
})
```

Map validation checks one attribute at a time and has no view of its
siblings, so a conditional presence rule cannot read the field it names;
it fails closed rather than passing silently. Cross-field checks belong
in `AfterValidation`.

## Other Laravel rules

| Rule | Meaning |
| --- | --- |
| `confirmed` | must equal the `<Field>Confirmation` sibling |
| `prohibited` | must be absent or empty |
| `required_if=Kind company` | required when another field has a value |
| `required_unless=Kind individual` | required unless another field has a value |
| `required_with=Email` / `required_without=Email` | required alongside/without another field |

## Manual validation

```go
validator := validation.New()
result := validator.Validate(&input)
if result.Fails() {
    messages := result.Messages()   // map[string][]string
    first := result.First()
}

// Single values and maps
err := validator.ValidateValue(email, "required,email")
result = validator.ValidateMap(data, map[string]string{"name": "required"})
```

## Custom rules and messages

```go
validator.RegisterValidation("even", func(fl validator.FieldLevel) bool {
    return fl.Field().Int()%2 == 0
})

validator.SetMessages(map[string]string{
    "required": ":field is mandatory",
})
validator.SetAttributeNames(map[string]string{
    "email": "e-mail address",
})
```

Rules come from [go-playground/validator](https://github.com/go-playground/validator);
all of its tags (`uuid`, `url`, `oneof=`, `gtfield=`, ...) are available.
