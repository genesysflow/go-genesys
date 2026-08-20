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
