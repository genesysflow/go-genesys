# Console

## Built-in commands

| Command | Purpose |
|---|---|
| `serve [--port]` | Start the HTTP server |
| `migrate` | Run pending migrations (`--dump-schema` on by default) |
| `migrate:rollback` | Rollback the last batch |
| `migrate:reset` | Rollback everything |
| `migrate:fresh` | Reset then re-run all migrations |
| `migrate:status` | Show migration state |
| `db:seed [--seeder name]` | Run seeders |
| `queue:work [--queue --tries --sleep --once --stop-when-empty]` | Process jobs |
| `queue:failed` / `queue:retry <id\|all>` | Manage failed jobs |
| `schedule:run` / `schedule:work` / `schedule:list` | Task scheduling |
| `key:generate [--show]` | Generate `APP_KEY` |
| `route:list` | Print all routes |
| `about` | Application/environment summary |
| `db:schema:dump` | Dump the schema to SQL |
| `sqlc:generate` | Run sqlc codegen (requires the `sqlc` binary on PATH) |
| `migrate:refresh [--seed]` | Roll back through each Down, then migrate again |
| `db:wipe [--force]` | Drop every table (refuses production without `--force`) |
| `db:show` / `db:table <table>` | Connection summary, tables and columns |
| `cache:clear [--store]` / `cache:forget <key>` | Flush or drop cache entries |
| `config:show <key> [--show-secrets]` | Print a config section (credentials masked) |
| `storage:link [--force]` | Link `public/storage` to `storage/app/public` |
| `schedule:test <task>` | Run one scheduled task immediately |
| `event:list` | Show registered event listeners |
| `queue:monitor <queues> [--max]` | Report queue sizes and backlogs |
| `queue:restart` | Ask workers to restart after their current job |
| `down` / `up` | Maintenance mode |

Laravel's `config:cache`, `route:cache`, `view:cache` and `optimize` have
no analogue here and are deliberately absent rather than faked: configuration
is parsed once at boot, and routes and views are Go code compiled into the
binary.

## Generators

```
make:controller [-r]   make:model        make:migration
make:middleware        make:provider     make:job
make:event             make:listener     make:seeder
make:request           make:command      make:policy
make:mail              make:notification make:factory
make:resource          make:rule         make:observer
make:cast              make:scope        make:channel
make:exception         make:enum         make:test
make:view              make:component    make:auth
```

Generated files land in the conventional directories (`app/jobs`,
`app/events`, `database/seeders`, ...). `make:view` and `make:component`
take dot-notation names (`users.index`); every other generator takes a
type name.

### Authentication scaffolding

`make:auth` writes the login, registration, logout and password-reset
flows - controllers, form requests, routes and views - as ordinary
application code:

```
app/http/auth/controller.go
app/http/auth/requests.go
app/http/auth/routes.go
resources/views/auth/{login,register,forgot_password,reset_password}.html
```

Wire it up where you register routes; the framework does not know your
user model, so registration goes through a `Create` hook:

```go
controller := &auth.Controller{
    Guard: sessionGuard,
    Users: userProvider,
    Create: func(email, hashed string) (genesysauth.Authenticatable, error) {
        user := &models.User{Email: email, Password: hashed}
        return user, database.Create(user)
    },
}
controller.Routes(router)
```

## Custom commands

```go
app.Register(&console.ConsoleServiceProvider{
    AppName: "myapp",
    Routes:  routes.Register,
    Commands: func(root *cobra.Command) {
        root.AddCommand(myconsole.SyncOrdersCommand(app))
    },
})
```

`genesys make:command SyncOrders` scaffolds the command file.

## Project scaffolding

```bash
genesys new myapp      # full project skeleton
genesys upgrade        # refresh framework scaffolding
```

## Writing a command

A command declares its arguments and options, and talks to the user
through a context:

```go
var Import = &console.Command{
    Name:        "orders:import",
    Description: "Import orders from a CSV",
    Arguments: []console.Argument{
        {Name: "file", Description: "The CSV to import", Required: true},
    },
    Options: []console.Option{
        {Name: "dry-run", Description: "Parse without writing"},
        {Name: "chunk", Description: "Rows per batch", Default: "500"},
        {Name: "since", Description: "Only rows after this date", TakesValue: true},
    },
    Handle: func(c *console.Context) error {
        if !c.Confirm("Import "+c.Argument("file")+"?", true) {
            c.Line("Aborted.")
            return nil
        }

        c.Table([]string{"File", "Chunk"}, [][]string{
            {c.Argument("file"), c.Option("chunk")},
        })

        return c.WithProgressBar(len(rows), func(advance func()) error {
            for range rows {
                advance()
            }
            return nil
        })
    },
}

kernel.AddCommand(Import.Cobra(app))
```

An option with a `Default` takes a value; one without is a boolean
switch, read with `c.BoolOption`. An option that takes a value but has no
sensible default says so with `TakesValue`, otherwise `--since 2020-01-01`
would parse the date as a positional argument.

Output helpers: `Line`, `Linef`, `Info`, `Comment`, `Warn`, `Error`,
`NewLine`, `Table`, `WithProgressBar`. Prompts: `Ask`, `Secret`,
`Confirm`, `Choice`. `c.Call("db:seed")` runs another command.

Colour is suppressed when output is not a terminal (and under
`NO_COLOR`), so logs stay plain. Prompts honour `--no-interaction` and
take their default, so a command in a cron job never blocks on input that
will not arrive; `Secret` disables terminal echo.
