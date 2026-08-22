# Changelog

All notable changes to Go-Genesys are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow SemVer.

## [Unreleased]

### Security - round 8: an adversarial pass over the framework

Most of what was probed held: operators are allowlisted, identifiers are
quoted, `Back` refuses to leave the host, tokens are hashed and compared
in constant time, templates escape by default, CORS panics on the
wildcard-plus-credentials combination, and the dev panel refuses to
mount in production. Six things did not.

- **`Select` emitted client SQL.** A column containing parentheses was
  passed through as an expression, so a query built from a client-chosen
  field list could smuggle in a subquery. `Select`/`AddSelect` now quote
  every column as the identifier it is; **`SelectRaw` is the explicit
  door for an expression**, and the aggregates use it. A caller passing
  `COUNT(*)` to `Select` must move it to `SelectRaw`.
- **A component slot rendered a plain string as raw HTML**, which is
  stored XSS behind an invisible opt-in: a slot is usually filled from a
  handler, and a handler's data is a request's data. A string is now
  escaped; markup says so by arriving as `template.HTML`, which is what
  `raw` produces. A template passing literal markup to a slot needs
  `(raw "...")`.
- **`ctx.Bind` filled a model's primary key and timestamps from the
  request** - enough to write over another row. Those columns are
  cleared after binding, through the new `database.ServerOwned`.
- **The scaffolded login was unthrottled.** `make:auth` now rate-limits
  the endpoints that check a credential, keyed by address as well as IP
  so one attacker cannot lock every account out and one office NAT is
  not one bucket. `Controller.Limiter` takes the shared cache store.
- **There was no safe way to honour a "next" URL**, so applications
  write `RedirectTo(ctx.Query("next"))` and ship an open redirect.
  **`ctx.Intended(fallback)`** is Laravel's answer: the auth middleware
  records where a guest was headed, and Intended honours it only when it
  stays on this host.
- **`APP_DEBUG` could not be turned on.** The flag was read from the
  environment before configuration existed, so `debug: true` in
  `config/app.yaml` did nothing and the debug error page was
  unreachable. Configuration now wins where it says something, with the
  environment variable as the fallback and off as the default.

The session provider warns when a production application hands out a
cookie that is not Secure. The example gains the security headers it was
not sending, scopes CORS to the JSON API rather than the session-backed
HTML site, and defaults both `session.secure` and `app.debug` to the
safe value so a deployment that forgets to say is not the one that
leaks.

Parameter binding - the SQL feature that actually stops injection - was
already used for every value the query layer sends, and is now pinned by
tests: seven injection payloads round-trip as data through every value
clause, an INSERT and an UPDATE; the MySQL DSN is asserted not to carry
`interpolateParams` (which replaces server-side binding with client-side
escaping) or `multiStatements` (which lets one call run several). The
tests also record what binding does *not* buy: `modernc.org/sqlite`
executes a trailing statement even when the call has bindings, so a
mistake in the SQL runs rather than misreads - which is the reason
identifiers are quoted instead of trusted.

Forty security tests cover all of it, and the classes that already held:
IDOR across every write route, mass assignment through both the form and
the JSON API, session fixation on login and logout, cookie flags,
account enumeration, token forgery, traversal, and what an error page
and a 403 are allowed to say.

### Added - round 7: an example application, and what it exposed

The framework had grown a lot of Laravel-shaped surface with no
application exercising it end to end. `example/` is now a small blog -
authentication, policies, form requests, morph relations, a queued job,
a token API, a console command and scheduled tasks - with fifty-seven
feature tests driving it through its real routes, middleware and database.
Building it found seventeen defects. Everything below is one of them.

- **View layouts**: `view.Manager.SetLayout` (and `view.layout` in
  configuration) wraps every rendered page in a layout, which receives
  the page as `{{.content}}`. `ctx.ViewIn(layout, name, data)` picks a
  different one, and an empty layout opts out - a fragment has no
  business carrying the site's chrome. Composition was `{{template}}`
  includes before this, which cannot wrap a page in anything.
- **The kernel installs the session middleware** from the registered
  session manager. Every application had to remember
  `kernel.UseFiber(manager.Middleware())` itself, which is how a login
  flow ends up working under test and failing in the browser.
  `KernelConfig.DisableSession` turns it off for an application that
  places the middleware itself.
- **`auth.ResolveUser(guard)`**: the user a public page still needs to
  know about - to greet a reader, show an edit link, or let a policy see
  a draft its author is entitled to - without requiring one. Guarding a
  route is still `auth.Middleware`'s job.
- **`auth.CanBy[T](gate, ability, param, column)`**: the `can` middleware
  for a route addressed by a slug, uuid or email rather than by its
  primary key.
- **`ctx.CreatedResource(data)`**: a 201 with the same `data` envelope
  `ctx.Resource` uses, so a client reads `data.*` whether it just wrote
  the record or fetched it.
- **`database.TableNameOf(value)`**: the table a value's model maps to,
  where `TableNameFor[T]` needs a type - which is what a polymorphic
  column has when all it holds is an interface.
- **`Manager.SetMailer`** on the notification manager, and a fallback to
  the application's default mailer when it was built without one. A
  manager built at boot held the mailer that existed then, so swapping in
  an array mailer for a test never reached the mail channel.
- **`console.Option.TakesValue`**: an option that takes a value but has
  no sensible default. Without it `--since 2020-01-01` parsed the date as
  a positional argument, because an empty default meant "boolean flag".
- **`middleware.CSRFConfig.Except`**: paths exempt from verification, for
  an endpoint authenticated by bearer token rather than cookie - without
  a cookie there is no cross-site request to forge. An exempt path still
  gets a token issued, so a form rendered by one can post to a guarded
  path.
- **Every `make:*` Go stub is compiled against the framework in a test.**
  Only `make:auth` was, which is how a policy stub teaching a fragile
  comparison and a resource stub whose own doc example did not match its
  signature both survived.
- **`Session.HasOld(key)`**, answering by key rather than by comparing a
  value against a sentinel.
- **`mail.SendDefault(mailable)`**: sends through the application's
  mailer, resolved when the message is sent rather than when the handler
  was wired - the same reason the notification manager needed
  `SetMailer`.
- **`devtools.RegisterRoutes(router, recorder, path)`**: mounts the panel
  on a router, which is where applications register routes. It refuses in
  production for the same reason `Register` does.

### Fixed - round 7

- **A validation rule was applied to an attribute that was not
  submitted.** `ValidateMap` failed every rule on a key absent from the
  data, so `unique=posts.slug` reported "has already been taken" for a
  field nobody filled in. Rules about a value are now skipped for an
  absent attribute, as they are in Laravel; rules about presence still
  apply.
- **Form-request `Rules()` ran against the raw input**, so a value
  `PrepareForValidation` derived - a slug from a title - was never
  checked. They now run against the prepared request, with input the
  struct does not bind still visible to them.
- **Configured paths were resolved against the working directory**
  rather than the application root, so `resources/views` only worked when
  the binary ran from that directory. Views, translations, session files
  and the log file now all resolve against the base path; an absolute
  path is left alone.
- **`TestRequest.WithForm` concatenated fields instead of encoding
  them**, truncating any value containing a space, an ampersand or a plus
  sign - which is most form values worth testing.
- **A token's `tokenable_type` held a Go type string**, package path and
  all, so moving the model to another package would orphan every token
  ever issued. It now holds the model's table name, like every other
  polymorphic column in the framework.
- **`Attach`/`Detach`/`Sync` refused a `morphToMany` relation**, so a
  polymorphic pivot could be read but never written.
- **`OldInput.Has` compared against a sentinel string**, so a flashed
  value equal to that sentinel reported absent. It asks the session by
  key now.
- **The `make:policy` stub compared an `int64` id with the `any` an
  identifier is.** That compiles, and is false forever when the two hold
  different numeric types - a policy that denies everything with no
  error to find. The stub asserts the identifier instead, and says why.
- **The `make:resource` stub's doc example did not match its own
  signature**, so the first thing a developer copied out of it failed.
- **The scaffolded password reset did not reset the password.** The
  generated controller left the two things the flow exists for - mailing
  the link and storing the new password - as `TODO` comments, so a
  reset reported success and changed nothing. `Controller.SendLink` and
  `Controller.UpdatePassword` are now hooks the application fills in,
  like `Create` for registration, and `UpdatePassword` runs inside
  `Consume` so a token is spent only when the password really changed.

### Added - round 6: console, authorization, and the surrounding tooling

- **Console command base**: `console.Command` declares arguments and
  options; the handler talks to the user through a context carrying
  output helpers (`Line`/`Info`/`Warn`/`Error`/`Table`/`WithProgressBar`),
  prompts (`Ask`/`Secret`/`Confirm`/`Choice`), and `Call` for invoking
  another command. Output goes through the command's streams, so commands
  are testable; colour is suppressed off-terminal and under `NO_COLOR`;
  prompts honour `--no-interaction`, so a command in cron never blocks on
  input that will not arrive; `Secret` disables terminal echo.
- **New commands**: `migrate:refresh`, `db:wipe` (refuses production
  without `--force`), `db:show`, `db:table`, `cache:clear`,
  `cache:forget`, `storage:link`, `config:show` (credentials masked
  unless `--show-secrets`), `schedule:test`, `event:list`.
- **Fourteen `make:` generators** - mail, notification, factory,
  resource, rule, observer, cast, scope, channel, exception, enum, test,
  view, component - plus `make:auth`, which scaffolds the login,
  registration and password-reset flows. Every generated Go stub is
  compiled against the framework in a test, not just parsed.
- **Policies**: `auth.RegisterPolicy[Post](gate, &PostPolicy{})` binds a
  policy struct to a model type; the gate answers by model, kebab-casing
  method names so `ViewAny` answers `"view-any"`. Registration rejects a
  bool-returning method with the wrong arguments, since that typo would
  otherwise deny silently forever.
- **Request authorization**: `ctx.Can`/`Cannot`/`Authorize` and
  `auth.GateMiddleware`, plus the `auth.Can[T]`/`auth.CanAny[T]` route
  middleware. With no gate bound the request fails closed. Views receive
  the gate as `.gate`.
- **Personal access tokens**: a Sanctum-shaped table, repository and
  guard. Only the SHA-256 of the secret is stored, compared in constant
  time; abilities, expiry, `RevokeAll` and `PruneExpired` included, with
  `auth.RequireAbility` for guarding a route.
- **Scheduler**: more frequencies, `Timezone`/`In`, `When`/`Skip`/
  `Between`/`UnlessBetween`/`Environments`, `Exec` for shell commands
  (with output capture), `Job` for queue dispatch, before/after/success/
  failure hooks, and `OnOneServer` backed by a shared cache lock.
- **Mailables**: `mail.Mailable` (Envelope/Content/Attachments) with
  `mail.Send`, `mail.SendWith`, and `mail.To(...).Send(mailable)`.
- **Notification channels**: broadcast and webhook (a Slack or Teams
  incoming webhook is a URL to POST to), `Manager.Extend` for channels of
  your own, and `notifications.NewFake` with assertions.
- **Model layer**: `morphTo` with a morph registry, `ToMap`/`ToMapSlice`
  honouring Hidden/Visible/Appends, `Fresh`/`Refresh`/`Replicate`/`Is`,
  and factory `State`/`Sequence`.
- **Testing**: `TestCase.ActingAs`/`ActingAsGuest`, a cookie jar so a
  session survives a redirect, `AssertJsonMissing`/`AssertHeaderMissing`/
  `AssertCookie`/`AssertLocationContains`, and `testutil/dbtest` database
  assertions.
- **Support**: a fluent `Str.Of` chain and two dozen string helpers,
  slice/map helpers (MapSlice, Filter, Reduce, Unique, Chunk, GroupBy,
  KeyBy, Only, Except, Partition, ...), `Num` formatting, and
  `support.Pipe`.
- **Dev panel**: `devtools` records requests and queries in a ring buffer
  and serves them at `/_genesys`. It refuses to mount in production, does
  not record itself, and withholds query bindings unless asked.

- **Queue operations**: `queue:monitor` reports queue sizes and flags a
  backlog above `--max`; `queue:restart` leaves a signal that workers
  watching for it honour between jobs, so a deploy replaces workers
  without killing a job mid-flight.

### Changed - round 6

- **`Queue.Size` returns `(int64, error)`** on every driver. The memory
  and redis drivers returned a bare `int`, swallowing errors - a monitor
  reading an unreachable queue as empty is worse than one that says it
  cannot tell - and the inconsistency made a common `SizeProvider`
  interface impossible.

### Fixed - round 6

- **`Response.RedirectRoute`** cannot resolve route names (it holds no
  router) and is documented as deprecated in favour of
  `ctx.RedirectToRoute`.
- **`make:policy`** generated `gate.Define` calls rather than a policy,
  and did not suffix the type name.

### Added - round 5: the web request lifecycle

- **Request ergonomics**: `ctx.User()`/`SetUser()`/`HasUser()` with the
  typed `http.UserAs[T]` and `auth.UserFrom(ctx)`; `ctx.Session()`,
  `ctx.Old()`, `ctx.Errors()`; and `ctx.Route()`/`RouteName()`/`RouteIs()`
  with `users.*` wildcards. `auth.Middleware` resolves the user once per
  request instead of every handler re-resolving it.
- **Fluent redirects**: `ctx.Back()`/`RedirectTo()`/`RedirectToRoute()`
  return a builder with `WithErrors()`, `WithInput()`, `With()`, and
  `Status()`. `WithErrors` accepts `map[string][]string`, an
  `*errors.ValidationError`, a `*support.MessageBag`, or a plain error;
  `WithInput()` never flashes password fields.
- **Validation failures branch on the client**: API clients keep the 422
  envelope, browsers are redirected back to the form with the messages and
  their old input flashed. Without a session the 422 shape is kept rather
  than a redirect that loses the errors.
- **Shared view data**: `ctx.View` layers `errors`, `old`, `session`,
  `user`, and `csrf_token` under the handler's own data.
- **Form request lifecycle**: optional `PrepareForValidation`,
  `Authorize`, `Rules`, `Messages`, `Attributes`, and `AfterValidation`.
  Authorization runs before validation; per-request messages go through
  `Validator.WithOverrides` rather than mutating the shared validator.
- **Database and conditional rules**: `unique=users.email` (with Laravel's
  ignore argument, `unique=users.email.42[.column]`), `exists=users.email`,
  `confirmed`, `prohibited`, and Laravel-shaped messages for
  `required_if`/`unless`/`with`/`without`. Both database rules fail closed
  when no connection is available, and only accept plain identifiers.
- **View helpers and composers**: `route`, `url`, `asset`, `config`,
  `trans`, `csrf_field`, `method_field`, plus
  `Manager.Composer("partials.*", fn)`. Helpers resolve their dependency
  at render time, so provider order does not matter, and degrade to empty
  rather than failing to parse when unwired.
- **`support.MessageBag`**: `Has`/`First`/`Get`/`All`/`Any`/`Keys`, the
  bag flashed on validation failure and shared with views.

### Fixed - round 5

- **Route middleware chained after registration never ran**:
  `GET(path, h).Middleware(auth.Middleware(guard))` captured the middleware
  slice by value at registration, so appending to it silently left the
  route unprotected. Route middleware is now read at request time.
- **Sessions were discarded when a handler returned an error**: the
  middleware returned early without saving, so anything written while
  handling a failing request was lost.
- **`Request.All()`/`Input()` ignored JSON bodies**: for a JSON API
  request - where the body is all the input there is - they reported no
  input at all.
- **`Response.RedirectRoute` cannot resolve route names** (it holds no
  router) and is documented as deprecated in favour of
  `ctx.RedirectToRoute`.

### Added - round 4: closing the core parity gaps

- **Polymorphic relations**: `morphOne`/`morphMany`/`morphToMany` via
  `rel:"morphMany,as:commentable"`; the morph type column stores the
  parent's table name. Eager loading, `Has`/`WhereHas`, and `WithCount`
  all honour the morph type.
- **Through relations**: `hasManyThrough`/`hasOneThrough` via
  `rel:"hasManyThrough,through:users"`, with batched two-hop eager
  loading and joined existence queries.
- **Relation counts**: `Query[T]().WithCount("Posts")` fills a
  `PostsCount int64 \`db:"-"\`` field with one grouped query per
  relation, across every relation kind.
- **Attribute casting**: `db:"settings,json"` marshals any field to a
  JSON column and back; `db:"ssn,encrypted"` (string fields) encrypts
  at rest with the application key - the struct always holds plaintext,
  and dirty tracking stays exact. Wired automatically by the
  EncryptionServiceProvider.
- **Write helpers**: atomic `Increment`/`Decrement` on model queries,
  bulk `Upsert` (ON CONFLICT / ON DUPLICATE KEY, with timestamp
  stamping) on the builder and `database.Upsert[T]`, and
  `database.Touch` to bump `updated_at`.
- **Query builder**: `LockForUpdate`/`SharedLock` (per driver),
  `Union`/`UnionAll` with cross-driver placeholder renumbering and
  ORDER BY applying to the combined result, `WhereJSON("meta",
  "specs.weight", ">", 10)` JSON-path constraints, and `CrossJoin`.
- **Read/write splitting**: `read_hosts` on a connection routes read
  queries round-robin over replica pools while writes and transactions
  stay on the primary.
- **Job timeouts**: `worker.Timeout` / a job's `Timeout()` bounds each
  attempt; timed-out jobs fail into the normal retry path, and
  `ContextJob` handlers receive the deadline through their context.
  `queue:prune-failed --hours` deletes old failed jobs on every driver.
- **Queued event listeners**: `events.ListenQueued` serializes events
  onto the queue for workers to replay; `Dispatcher.Subscribe` registers
  event subscribers.
- **Cache tags**: `cache.Tags(store, "users").Put/Get/Remember/Flush`
  with version-keyed invalidation that works across processes on shared
  stores.
- **Presence channels + multi-node broadcasting**: `presence-` channels
  track member info (`genesys:here`/`joining`/`leaving`), and
  `hub.ConnectRedis` joins hubs to a Redis pub/sub backplane so
  broadcasts reach every node.
- **Encrypted cookies**: `middleware.EncryptCookies(encrypter,
  except...)` - browsers hold ciphertext, handlers read plaintext,
  forged cookies are dropped.
- **HTTP client fake**: `client.NewFake().Respond(...)` with request
  recording and `AssertSent`/`AssertNothingSent` - Laravel's
  `Http::fake()`.
- **Mail manager**: named mailers with a default, a failover mailer
  chain, and `mail.SendQueued` for worker-delivered messages.
- **Validation wildcards**: `items.*.email` rules on `ValidateMap`,
  reporting failures under their element index.
- **Contextual container bindings**: `c.When("reports").Needs("disk").
  Give(...)` consulted by `MakeFor`.
- **Test scaffolding**: `filesystem.NewFakeDisk()` (Storage::fake with
  assertions) and a `router.Health()` liveness endpoint (Laravel 11's
  `/up`).

### Fixed - maintenance mode wiring

- **Maintenance mode was inert out of the box**: `genesys down` wrote the
  marker file, but neither the example app, the `routes.go` generated by
  `genesys new`, nor the fallback middleware stack in `serve` registered
  `middleware.Maintenance`, so every request still returned 200. All
  three now register it last in the global stack, with the path resolved
  against `app.BasePath()` so it matches where the command writes.

### Fixed - whole-project audit

A framework-wide adversarial review; the notable fixes, by area:

- **Queue durability**: the redis driver now parks popped jobs on a
  reserved set via atomic Lua scripts (Laravel's `retry_after`), so a
  worker crash can no longer lose a job; the database driver reclaims
  reservations older than `RetryAfter`; delayed-job migration is
  atomic; queue keys are namespaced so a queue named "failed" cannot
  collide with the failed list. MySQL `[]byte` columns decode correctly.
- **Batches**: a failing-then-retried job settles its batch exactly
  once, on the terminal attempt - counters no longer exceed the total
  and `Catch` no longer fires early. `WithoutOverlapping` postpones a
  blocked job instead of burning attempts; `ReleaseUniqueLock`
  middleware frees a unique job's slot the moment it completes.
  `RegisterJob`'s explicit names now round-trip through serialization.
- **Schema**: table-level `Unique`/`Primary`/`Index` and column-level
  `.Index()` now actually reach the generated SQL (previously silently
  dropped); a real MySQL grammar (backticks, `AUTO_INCREMENT`,
  `MODIFY COLUMN`) replaces the SQLite fallthrough.
- **ORM/query**: `Has`/`WhereHas` correlate self-referential relations
  through an alias; `Count` is correct over `DISTINCT`/`GROUP BY`;
  `Skip` without `Take` emits valid SQL on every driver; `Chunk` and
  cursor pagination enforce their keyset ordering; an outer field now
  overrides an embedded model's column; the `Where`/`Having` operator
  position is whitelisted (SQL-injection hardening); MySQL updates
  count matched rows (`clientFoundRows`) so no-op updates are not
  mistaken for missing rows; error-state connections surface their
  error from `QueryRow` instead of panicking, and concurrent first
  connections no longer leak a pool.
- **Security**: login timing equalisation burns a hash at the real
  default cost; signed URLs canonicalise with escaping so values
  containing `&`/`=` cannot be re-partitioned into smuggled parameters;
  remember-me tokens are stored SHA-256-hashed; CORS refuses the
  wildcard-plus-credentials combination; maintenance-mode secrets
  compare in constant time and the bypass cookie is Secure/SameSite;
  mail rejects addresses containing CRLF and strips newlines from all
  header values (header-injection hardening).
- **Foundation**: configuration is loaded before provider registration
  (so providers can read config in `Register`, as scaffolded apps do);
  deferred providers no longer boot half-initialised; nested container
  resolution can no longer deadlock against concurrent registration;
  `serve` reuses an already-registered RouteServiceProvider, honours
  `--host`, and gracefully shuts down for 10 seconds (previously 10ns).
- **Services**: loggers apply their level to zerolog (Info-default no
  longer emits debug records) and are race-safe; lang placeholders
  apply longest-first; cache lock release is an atomic compare-and-
  delete on memory/redis stores; the HTTP client strips custom headers
  on cross-host redirects; `Str.Limit` is multibyte-safe;
  `Collection.Skip` tolerates negative counts; generator names are
  stripped of path separators; `genesys new` scaffolds from embedded
  templates; `genesys upgrade` fetches `@latest` instead of pinning to
  the CLI's own version; notifications populate `CreatedAt`/`ReadAt`;
  the broadcast hub closes a dropped client's connection; the scheduler
  no longer skips minutes while a slow task runs.

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
