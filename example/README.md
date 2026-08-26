# The example application

A small blog, built to exercise the framework rather than to be a
starting template. Every feature the framework offers is used here by
something that has a reason to use it, and a feature test drives it
through the real routes, middleware and database.

It is the first place to look for how a piece fits together, and the
first place a regression shows up.

## Running it

```sh
cp .env.example .env
go run . key:generate     # writes APP_KEY, without which CSRF stays off
go run . migrate
go run . db:seed
go run . serve            # http://localhost:3000
```

Sign in as the seeded editor with `editor@example.com` / `password123`,
or register a new account. Authors write; editors publish.

The tests need none of this - each one boots the application against its
own in-memory database:

```sh
go test ./tests/...
```

## What the blog is

A post belongs to an author, has comments, carries tags and can have
files attached. An author writes a draft; an editor publishes it, which
queues the work of notifying the author. Readers see published posts;
a draft is visible to its author and to editors.

Four models, in `app/models`:

| Model        | Relations                                                  |
|--------------|------------------------------------------------------------|
| `Post`       | `belongsTo` author, `hasMany` comments, `morphToMany` tags, `morphMany` attachments |
| `Comment`    | `belongsTo` post, `morphMany` attachments                   |
| `Tag`        | attached polymorphically, so it can label other models later |
| `Attachment` | `morphTo` its owner - a post or a comment                   |

## Where each thing lives

**Routing and HTTP** — `routes/blog.go` registers everything: the
scaffolded authentication flow, the HTML pages, and a JSON API under
`/api/blog`. Each writing route carries the policy that governs it, so
the handler never runs for a caller who may not.

**Form requests** — `app/http/blog/requests.go` uses the whole
lifecycle: `PrepareForValidation` derives the slug from the title,
`Authorize` refuses a caller who may not write at all, `Rules` adds the
uniqueness check only the database can answer, `Messages` and
`Attributes` reword the failures, and `AfterValidation` makes the
cross-field check no single rule can.

**Validation redirects** — a browser that fails validation is sent back
to the form with its input and messages; a JSON client gets a 422. Both
paths are covered in `tests/posts_test.go` and `tests/api_test.go`.

**Authorization** — `app/policies/post_policy.go` decides who may view,
update, delete and publish. `app/providers/blog_service_provider.go`
registers it and adds the `Before` hook that lets editors past
everything, so no policy has to know about the role.

**Authentication** — the scaffolding `make:auth` generates, in
`app/http/auth`, wired to this application's user model in
`routes/blog.go`: registration greets the new author, and the password
reset mails a link and writes the new password. Sessions for the site;
personal access tokens for the API, issued from `/api-tokens` and
checked with `RequireAbility`. `app/models/current_user.go` holds the
one narrowing the framework cannot do for you: `models.CurrentUser(ctx)`
is this application's `$request->user()`, and `models.AsUser` narrows
what a gate, a policy or the password broker is handed - so no handler
type-asserts, and none panics for a guest.

**Mailables** — `app/mail/welcome.go`: the welcome greeting and the
password-reset link, sent as objects rather than messages assembled by
hand.

**Queues, mail and notifications** — publishing dispatches an event; the
listener pushes `app/jobs/publish_post.go` onto the queue; the job
notifies the author by mail and records the notification in the
database. `app/jobs/weekly_digest.go` is the scheduled counterpart.

**Console** — `app/console/blog_stats.go` is `blog:stats`: options, a
table, cached work and a `--since` filter.

**Scheduling** — `app/tasks/tasks.go`, wired in `bootstrap/app.go`.
Pruning stale drafts runs at night, in production only, on one server
when several run the schedule; the digest is queued rather than run.

**Views** — `resources/views`. Pages render inside
`layouts/blog.html`, configured as the default layout in
`config/view.yaml`; a tag renders through the `components/tag.html`
component, so its markup lives in one place.

**Factories and seeders** — `database/factories` builds models for both
the seeders and the tests, with states (`Editors`, `Published`) for the
variations.

**Development panel** — `routes/devtools.go` mounts `/_genesys`, which
lists the requests handled and the queries they ran. It refuses to
mount in production.

## The tests

`tests/harness_test.go` boots the real application - every provider, the
real routes, the real middleware - against a throwaway database, and
swaps in an array mailer and an in-memory queue so a test can read what
the application would have sent and dispatched.

Nothing in the harness wires the application up. It builds the kernel
exactly as `serve` does, so a test cannot pass on wiring the application
does not have.

| File                 | What it covers                                        |
|----------------------|-------------------------------------------------------|
| `auth_test.go`       | registration, login, logout, guests, CSRF             |
| `password_reset_test.go` | the reset link, the token, the new password       |
| `posts_test.go`      | CRUD, validation redirects, slug uniqueness, policies |
| `api_test.go`        | tokens, abilities, JSON errors, hidden fields         |
| `queue_test.go`      | the publish job, its mail and database notifications  |
| `console_test.go`    | `blog:stats` and the scheduled tasks                  |
| `relations_test.go`  | tags, attachments, counts, serialization              |
