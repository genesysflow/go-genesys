# Collections

Generic fluent collections over slices.

```go
import "github.com/genesysflow/go-genesys/support/collection"

c := collection.Collect(3, 1, 4, 1, 5)
// or wrap an existing slice
c = collection.FromSlice(numbers)
```

## Chaining (same element type)

```go
result := collection.Collect(1, 2, 3, 4, 5, 6).
    Filter(func(n int) bool { return n%2 == 0 }).
    Map(func(n int) int { return n * n }).
    Reverse().
    Take(2).
    All() // []int{36, 16}
```

Available: `Filter`, `Reject`, `Map`, `Each`, `Take` (negative = from the
end), `Skip`, `Chunk`, `Reverse`, `SortBy`, `Push`, `Concat`, `Tap`,
`First`, `FirstWhere`, `Last`, `Contains`, `Every`, `Len`, `IsEmpty`.

## Type-changing helpers

Go methods cannot introduce new type parameters, so transforms that change
the element type are package functions:

```go
users := collection.FromSlice(userSlice)

names := collection.Pluck(users, func(u User) string { return u.Name })
totals := collection.Sum(users, func(u User) int { return u.Age })
byTeam := collection.GroupBy(users, func(u User) string { return u.Team })
byID := collection.KeyBy(users, func(u User) int64 { return u.ID })
unique := collection.Unique(users, func(u User) string { return u.Email })
lengths := collection.Map(users, func(u User) int { return len(u.Name) })

csv := collection.Reduce(users, "", func(carry string, u User) string {
    if carry == "" {
        return u.Name
    }
    return carry + "," + u.Name
})
```

## String helpers

```go
support.Str.Plural("user")            // users
support.Str.PluralCount("user", 1)    // user
support.Str.Before("ada@x.com", "@")  // ada
support.Str.After("ada@x.com", "@")   // x.com
support.Str.Between("a[b]c", "[", "]")// b
support.Str.Mask("ada@example.com", '*', 4)  // ada@***********
support.Str.Squish("  a   b ")        // "a b"
support.Str.Take("héllo wörld", 5)    // héllo
```

Character-based, not byte-based: a multi-byte string is never cut
mid-character.

Chained, a transformation reads as one statement:

```go
support.Str.Of("  Hello World  ").
    Trim().
    Lower().
    Replace(" ", "-").
    Value()   // hello-world
```

## Slice and map helpers

```go
support.MapSlice(users, func(u User) string { return u.Email })
support.Filter(numbers, func(n int) bool { return n > 0 })
support.Reduce(numbers, 0, func(carry, n int) int { return carry + n })
support.Unique(tags)
support.Chunk(rows, 500)
support.GroupBy(users, func(u User) string { return u.Team })
support.KeyBy(users, func(u User) int64 { return u.ID })
support.Only(payload, "name", "email")
support.Except(payload, "password")     // copies; the source is untouched
support.Partition(users, func(u User) bool { return u.Active })
```

## Numbers

```go
support.Num.Format(1234567)          // 1,234,567
support.Num.Currency(1234.56, "$")   // $1,234.56
support.Num.Percentage(12.5, 0)      // 13%
support.Num.FileSize(1536)           // 1.5 KB
support.Num.Ordinal(11)              // 11th
support.Clamp(10, 0, 5)              // 5
```

Displayed numbers round half away from zero, so 12.5% shown to no
decimals is 13% rather than Go's default 12%.

## Pipeline

```go
body, err := support.Pipe(raw).
    Through(normalize, sanitize, validate).
    Run()
```

A stage that fails stops the pipeline: the stages after it never see a
value the failing stage did not produce.
