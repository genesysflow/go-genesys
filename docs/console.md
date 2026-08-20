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

## Generators

```
make:controller [-r]   make:model        make:migration
make:middleware        make:provider     make:job
make:event             make:listener     make:seeder
make:request           make:command      make:policy
```

Generated files land in the conventional directories (`app/jobs`,
`app/events`, `database/seeders`, ...).

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
