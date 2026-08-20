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

A failed validation returns an `*errors.ValidationError`, which the
framework error handler renders as Laravel's 422 shape:

```json
{
  "message": "The given data was invalid.",
  "errors": {
    "name": ["Name must be at least 3 characters"],
    "email": ["Email must be a valid email address"]
  }
}
```

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
