# Database

## Configuration

```yaml
# config/database.yaml
default: pgsql
connections:
  pgsql:
    driver: pgsql
    host: ${DB_HOST:localhost}
    database: ${DB_DATABASE:myapp}
    username: ${DB_USERNAME:postgres}
    password: ${DB_PASSWORD:}
  sqlite:
    driver: sqlite
    database: storage/database.sqlite
```

## Query builder

```go
import "github.com/genesysflow/go-genesys/facades/db"

rows, err := db.Table("users").Get()                  // []map[string]any
row, err := db.Table("users").Where("id", 1).First()  // query.ErrNoRows when missing

rows, err = db.Table("users").
    Select("id", "name").
    Where("age", ">", 18).
    OrWhere("admin", true).
    WhereIn("role", "editor", "admin").
    WhereNull("deleted_at").
    WhereGroup(func(q *query.Builder) {          // WHERE ... AND (x OR y)
        q.Where("a", 1).OrWhere("b", 2)
    }).
    Join("teams", "users.team_id", "=", "teams.id").
    GroupBy("teams.name").
    Having("count", ">", 3).
    OrderByDesc("created_at").
    Limit(10).Offset(20).
    Get()

// Aggregates and helpers
count, err := db.Table("users").Count()
avg, err := db.Table("orders").Avg("total")
names, err := db.Table("users").Pluck("name")
exists, err := db.Table("users").Where("email", email).Exists()

// Writes
err = db.Table("users").Insert(map[string]any{"name": "Ada"})
id, err := db.Table("users").InsertGetID(map[string]any{"name": "Ada"})
n, err := db.Table("users").Where("id", 1).Update(map[string]any{"name": "Grace"})
n, err = db.Table("logins").Where("user_id", 1).Increment("count")
n, err = db.Table("users").Where("active", false).Delete()

// Pagination
page, err := db.Table("users").OrderBy("id").Paginate(2, 15)
// page.Data, page.Total, page.PerPage, page.CurrentPage, page.LastPage
```

## ORM

Embed `database.Model` to get `ID`, `CreatedAt`, `UpdatedAt`:

```go
type User struct {
    database.Model
    Name  string `db:"name" json:"name"`
    Email string `db:"email" json:"email"`
}
```

Table names are inferred (`User` → `users`, `BlogPost` → `blog_posts`);
override with `func (User) TableName() string`.

```go
// CRUD (timestamps and ID handled automatically)
user := &User{Name: "Ada", Email: "ada@example.com"}
err := database.Create(user)
err = database.Save(user)         // insert when ID == 0, else update
err = database.Update(user)
err = database.Delete[User](user.ID)

found, err := database.Find[User](1)        // database.ErrNotFound when missing
first, err := database.FirstWhere[User]("email", "ada@example.com")
all, err := database.All[User]()

// Typed queries
adults, err := database.Query[User]().
    Where("age", ">=", 18).
    OrderBy("name").
    Get()

page, err := database.Query[User]().Latest().Paginate(1, 15) // typed paginator
```

## Transactions

```go
err := db.Transaction(func(tx contracts.Transaction) error {
    if _, err := tx.Exec("UPDATE accounts SET balance = balance - 100 WHERE id = $1", from); err != nil {
        return err
    }
    _, err := tx.Exec("UPDATE accounts SET balance = balance + 100 WHERE id = $1", to)
    return err // non-nil rolls back
})
```

## Migrations

```go
func (m *CreateUsersTable) Up(builder *schema.Builder) error {
    return builder.Create("users", func(table *schema.Blueprint) {
        table.ID()
        table.String("name", 255)
        table.String("email", 255).Unique()
        table.ForeignID("team_id")
        table.Foreign("team_id").References("id").On("teams").CascadeOnDelete()
        table.Index("email")
        table.Timestamps()
        table.SoftDeletes()
    })
}
```

Foreign key tables are inferred when omitted (`user_id` → `users.id`).
Run with `migrate`, undo with `migrate:rollback` / `migrate:reset`, rebuild
with `migrate:fresh`.

## Seeders & factories

```go
var userFactory = database.NewFactory(func(i int) *User {
    return &User{Name: fmt.Sprintf("User %d", i), Email: fmt.Sprintf("u%d@x.io", i)}
})

app.Register(&providers.SeedServiceProvider{
    Define: func(r *seed.Runner) {
        r.AddFunc("users", func() error {
            _, err := userFactory.Create(10)
            return err
        })
    },
})
```

Run all seeders with `db:seed`, or a subset with `db:seed --seeder users`.
