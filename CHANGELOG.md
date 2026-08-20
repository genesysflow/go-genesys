# Changelog

All notable changes to Go-Genesys are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow SemVer.

## [Unreleased]

### Added
- **ORM transactions**: `database.WithinTransaction(fn(tx *TxScope))`
  with an optional trailing scope on every ORM helper (and
  `AttachScoped`/`LoadScoped`-style variants), so creates, updates,
  deletes, soft deletes, and pivot writes are all-or-nothing.
  Previously ORM writes inside a transaction callback silently escaped
  it.
- **FirstOrCreate / UpdateOrCreate** (scope-aware; update keeps the
  found row's id and creation timestamp).
- **MySQL/MariaDB**: DSN building, driver mapping, backtick grammar
  normalization; apps register go-sql-driver/mysql with a blank import.
- **S3 temporary URLs**: `disk.TemporaryURL(ctx, path, ttl)` presigned
  GET links, with manager and storage-facade passthroughs.
- **Named queues**: `PushOn`/`LaterOn` across memory/database/redis
  drivers and priority draining via `worker.Queues` /
  `queue:work --queue=high,default`.
- **Templated mail**: `Message.ViewHTML`/`ViewText` render bodies
  through the view layer, deferring render errors to send time.
- **Queued notifications**: `RegisterQueued[T]` + `SendQueued` resolve
  channel routes at dispatch into a serializable job any worker can
  deliver.
- **Relation existence queries**: `Has`/`WhereHas`/`OrWhereHas`/
  `DoesntHave` with correlated EXISTS (pivot join for belongsToMany),
  dotted nested paths, and related-table constraints; query builder
  `WhereExists`/`WhereNotExists`/`WhereInSub` with cross-driver
  placeholder renumbering.
- **Relationship writes**: `Attach`/`Detach`/`Sync`/`Toggle` pivot
  management, `CreateFor` (hasOne/hasMany create with FK fill), and
  `Associate`/`Dissociate` for belongsTo. Reloading relations after a
  detach now clears stale fields.
- **Cursor pagination** (`CursorPaginate` on builder and ORM) and
  delete-safe keyset `Chunk`/`Each`.
- **Queue**: worker + per-job middleware (`worker.Use`,
  `WithoutOverlapping` over cache locks, `DispatchUnique` dedupe) and a
  database-backed `job_batches` repository making batch progress visible
  across processes (`queue.FindBatch`).
- **Broadcasting** (`broadcast`): WebSocket hub with public and
  authorized private channels, JSON subscribe protocol, slow-consumer
  protection, and a dispatcher bridge (`Broadcastable` events push to
  their channel). Dispatcher gains `ListenAll` wildcard listeners.
- **View components**: `{{component "alert" (dict ...)}}` renders
  templates under components/ with slot content via `{{slot .}}`.
- **Localization**: locale-pinned translator views
  (`translator.In("de")`), `DetectLocale` middleware
  (query/session/Accept-Language), and translated validation messages
  and attribute names via `validator.SetTranslator`.
- **Subdomain routing**: `router.Domain("{account}.example.com", ...)`
  with captured domain parameters and host fall-through.
- **Config validation**: `config.Validate`/`MustValidate` fail fast on
  missing keys at boot.
- **ORM relationships**: `rel` struct tags (hasOne/hasMany/belongsTo/
  belongsToMany) with Laravel-convention keys, batched eager loading via
  `With("Posts", "Posts.Tags")` (one query per relation, nested paths),
  and lazy `Load`/`LoadAll`.
- **Soft deletes**: `database.SoftDeletes` embed; queries hide trashed
  rows by default with `WithTrashed`/`OnlyTrashed`/`Restore`/
  `ForceDelete`; eager loads exclude trashed relations. Local query
  scopes via `Scope(fns...)`.
- **Model lifecycle**: Creating/Created/Updating/Updated/Saving/Saved/
  Deleting/Deleted hooks and `database.Observe` observers; dirty
  tracking (`IsDirty`/`GetDirty`/`GetOriginal`) with partial updates
  that only write changed columns.
- **Redis drivers** for cache (`RedisStore`), sessions (`RedisStorage`),
  and queue (`RedisQueue` with delayed jobs and a failed list), tested
  against miniredis.
- **Cache locks** (`cache.NewLock`, `WithLock`) and a cache-backed named
  `Throttle` middleware with X-RateLimit/Retry-After headers.
- **Testing**: `http.NewTestCase` request/assertion DSL, `queue.NewFake`
  with `AssertPushed[T]`, `dispatcher.Fake()` (Event::fake), and
  ArrayMailer assertion helpers.
- **Notifications** (`notifications`): mail + database channels,
  on-demand routes, unread feeds, `facades/notify`.
- **Auth**: password reset broker (hashed single-use tokens, throttling,
  expiry), signed email verification links, and remember-me persistent
  logins with token cycling on logout.
- **Queue bus**: `queue.Chain` (sequential jobs) and `queue.NewBatch`
  with `Then`/`Catch`/`Finally` callbacks and progress tracking.
- **Maintenance mode**: `genesys down`/`up` + `middleware.Maintenance`
  (503, Retry-After, secret bypass).
- **DB query listener**: `manager.Listen` fires QueryEvents with SQL,
  bindings, duration, and error for every query.
- **Helpers**: `support.Dump`/`Dd`, `support.DataGet` with wildcards,
  and the `support/faker` package for factories and seeders.
- **Query builder** (`query` package): fluent Where/Join/GroupBy/Having/
  OrderBy, aggregates, Pluck/Value/Exists, Insert/Update/Delete/Increment,
  per-driver grammar (postgres `$n`, mysql backticks), `Paginate`/
  `SimplePaginate`, and `db.Table(...)` entry points.
- **ORM** (`database`): embeddable `Model`, generic `All/Find/FirstWhere/
  Create/Save/Update/Delete`, typed `Query[T]()` with pagination, table
  name inference, automatic timestamps, struct scanning with `db` tags.
- **Collections** (`support/collection`): generic fluent collections with
  Map/Filter/Reduce/GroupBy/KeyBy/Pluck/Unique/Sum and more.
- **Cache**: file store, `Has/Add/Pull/Forever/Increment/Decrement`,
  `Remember`/`RememberForever`, config-driven stores, `facades/cache`.
- **Queue**: job serialization registry, memory and database drivers,
  worker with retries/backoff/panic recovery, delayed dispatch, failed-job
  storage, `queue:work`/`queue:failed`/`queue:retry` commands.
- **Sessions**: file and database storage drivers, correct one-request
  flash lifecycle, `FlashInput`/`Old` helpers.
- **Encryption** (`crypt`): AES-256-GCM keyed by `APP_KEY`, plus the
  `key:generate` command and `facades/crypt`.
- **Views** (`view`): html/template manager with dot-notation names,
  layouts/partials, shared data, dev reload, `ctx.View`.
- **Form requests**: `http.ValidateRequest[T]` with Laravel-shaped 422
  responses; `ctx.Resource`/`ctx.Paginated` response envelopes.
- **Authentication** (`auth`): session and token guards, ORM user
  provider, login/logout/attempt with session regeneration, middleware.
- **Authorization**: `auth.Gate` with Define/Allows/Denies/Authorize and
  before-hooks; `make:policy` scaffolding.
- **Scheduler** (`schedule`): cron expressions, fluent frequencies,
  overlap prevention, `schedule:run`/`schedule:work`/`schedule:list`.
- **Mail** (`mail`): MIME message builder, SMTP (TLS/STARTTLS), log and
  array drivers, `facades/mail`.
- **HTTP client** (`client`): fluent outbound requests with retries.
- **Localization** (`lang`): per-locale YAML files, `Trans`/`TransChoice`.
- **Signed URLs** (`urlsign`) and `middleware.ValidateSignature`.
- **Router**: `Fallback`, `Redirect`, middleware groups and aliases,
  `http.BindModel` route-model binding.
- **Events**: typed generic `events.Listen[T]` / `events.Emit`.
- **Error handling**: content negotiation (HTML error pages for browsers,
  JSON for APIs) with a debug page showing stack traces.
- **Schema builder**: fluent foreign keys with table inference.
- **Seeders & factories**: `seed.Runner`, `db:seed`, `database.NewFactory`.
- **Console**: `migrate:fresh`, `migrate:reset`, `route:list`, `about`,
  `db:seed`, `key:generate`, and `make:job/event/listener/seeder/request/
  command/policy` generators.
- **Facades**: cache, queue, event, log, session, config, route, view,
  auth, gate, crypt, mail, lang (alongside db and storage).
- **CI**: GitHub Actions workflow running vet and the race-enabled test
  suite.

### Hardening
- **Real-PostgreSQL integration tests**: the ORM end-to-end suite
  (CRUD/RETURNING ids, dirty tracking, nested eager loading, WhereHas
  through pivots, soft deletes, cursor pagination, chunking) and the
  connection-manager tests now run against PostgreSQL 16 - via
  `GENESYS_TEST_PG_*` env (a CI services container) or docker on
  demand, skipping when neither is available.
- **Benchmarks** for the hot paths (query compilation, row scanning,
  eager loading, create, dirty checks, WhereHas), smoke-run in CI so
  they cannot rot.
- **Example-app smoke test**: boots the full example application
  (providers, config, routes) and exercises live endpoints, catching
  wiring regressions unit tests miss.
- **CI**: gofmt check, PostgreSQL services container for the
  integration tests, benchmark smoke step, and a govulncheck job.

### Changed
- `sqlc:generate` now shells out to the `sqlc` binary on PATH instead of
  embedding the sqlc library, and passes flags through verbatim. Install
  it with `go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest`. The
  postgres test helper (`testutil.SetupPostgresContainer`) drives the
  docker CLI directly instead of testcontainers-go. Together this
  removes the embedded SQL parser, docker/containerd, grpc, wazero,
  opentelemetry, and testcontainers dependency trees (~60 modules),
  clearing the bulk of the dependency-audit findings.

### Fixed
- Documentation and example configs used `${VAR:default}` for env
  interpolation; the supported syntax is `${VAR:-default}`.
- Session flash data previously persisted forever; it now expires after
  one request as intended.
- `make:model` referenced a template that did not exist.
- `Router.Routes()` now includes routes registered in nested groups.
- Docker-dependent integration tests skip gracefully when no daemon is
  available instead of panicking the whole package.

## [1.1.0] and earlier

See git history.
