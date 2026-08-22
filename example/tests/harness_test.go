package tests

import (
	"path/filepath"
	"testing"

	"github.com/genesysflow/go-genesys/auth"
	"github.com/genesysflow/go-genesys/container"
	"github.com/genesysflow/go-genesys/database"
	"github.com/genesysflow/go-genesys/database/migrations"
	"github.com/genesysflow/go-genesys/example/app/models"
	"github.com/genesysflow/go-genesys/example/bootstrap"
	"github.com/genesysflow/go-genesys/example/database/factories"
	"github.com/genesysflow/go-genesys/foundation"
	genhttp "github.com/genesysflow/go-genesys/http"
	"github.com/genesysflow/go-genesys/http/middleware"
	"github.com/genesysflow/go-genesys/mail"
	"github.com/genesysflow/go-genesys/queue"
	"github.com/genesysflow/go-genesys/session"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

// harness is a booted copy of the real application, wired to a private
// in-memory database, with the pieces a test needs to assert on.
type harness struct {
	app    *foundation.Application
	kernel *genhttp.Kernel
	tc     *genhttp.TestCase
	mailer *mail.ArrayMailer
	jobs   *queue.MemoryQueue
	tokens *auth.TokenRepository

	// page is where the browser is: the Referer a form submission
	// carries, and what "back" means after a validation failure.
	page string
}

// boot builds the application exactly as `serve` does - every provider,
// the real routes, the real middleware - against a throwaway database.
// A wiring regression fails here, not in production.
func boot(t *testing.T) *harness {
	t.Helper()

	// Each test gets its own database. Shared-cache in-memory sqlite is
	// still one database per DSN, so the name carries the test's name.
	t.Setenv("DB_DATABASE", "file:"+t.Name()+"?mode=memory&cache=shared")
	t.Setenv("APP_ENV", "testing")
	t.Setenv("APP_KEY", "base64:dGVzdGluZy1rZXktMzItYnl0ZXMtbG9uZy0xMjM0NTY=")

	// The example's root, not the tests directory: config, views and
	// storage are addressed relative to it.
	app := bootstrap.App(appRoot(t))
	require.NoError(t, app.Boot())

	migrator := container.MustResolve[*migrations.Migrator](app)
	_, err := migrator.Run()
	require.NoError(t, err)

	// Mail goes to an array, not a mail server, so a test can read what
	// would have been sent.
	mailer := mail.NewArrayMailer()
	mail.SetDefaultMailer(mailer)

	// Jobs stay in memory until a test drains them, which is what makes
	// "the request queued the work" assertable.
	jobs := queue.NewMemoryQueue()
	queueManager := container.MustResolve[*queue.Manager](app)
	queueManager.Register("testing", jobs)
	queueManager.SetDefaultConnection("testing")

	kernelConfig := container.MustResolve[*genhttp.KernelConfig](app)
	kernelConfig.DisableStartupMessage = true

	kernel := genhttp.NewKernel(app, *kernelConfig)
	kernel.UseFiber(session.NewManager(session.Config{CookieSecure: false}).Middleware())

	routes := container.MustResolve[func(*genhttp.Router)](app)
	globalMiddleware := container.MustResolve[[]genhttp.MiddlewareFunc](app)
	kernel.Use(globalMiddleware...)
	routes(kernel.Router())

	// A browser always states what it accepts, and the framework reads
	// that to decide between an HTML redirect and a JSON error body. A
	// test driving the HTML site has to say the same thing, or it is
	// exercising the API's behaviour by accident.
	browser := genhttp.NewTestCase(t, kernel).
		WithHeader("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	h := &harness{
		app:    app,
		kernel: kernel,
		tc:     browser,
		mailer: mailer,
		jobs:   jobs,
		tokens: container.MustResolve[*auth.TokenRepository](app),
	}

	t.Cleanup(func() {
		manager := container.MustResolve[*database.Manager](app)
		_ = manager.Close()
		database.SetDefault(nil)
	})

	return h
}

// appRoot returns the example application's directory, which is the
// parent of this test package.
func appRoot(t *testing.T) string {
	t.Helper()

	root, err := filepath.Abs("..")
	require.NoError(t, err)
	return root
}

// author creates a signed-in-able author.
func (h *harness) author(t *testing.T) *models.User {
	t.Helper()
	user, err := factories.Users.CreateOne()
	require.NoError(t, err)
	return user
}

// editor creates a user who may act on anyone's post.
func (h *harness) editor(t *testing.T) *models.User {
	t.Helper()
	user, err := factories.Editors.CreateOne()
	require.NoError(t, err)
	return user
}

// post creates a draft by an author.
func (h *harness) post(t *testing.T, author *models.User) *models.Post {
	t.Helper()
	post, err := factories.Posts.CreateOne(func(p *models.Post) { p.AuthorID = author.ID })
	require.NoError(t, err)
	return post
}

// published creates a live post by an author.
func (h *harness) published(t *testing.T, author *models.User) *models.Post {
	t.Helper()
	post, err := factories.Published.CreateOne(func(p *models.Post) { p.AuthorID = author.ID })
	require.NoError(t, err)
	return post
}

// signIn drives the real login form, so a test's user arrives with the
// session a browser would have.
func (h *harness) signIn(t *testing.T, user *models.User) *models.User {
	t.Helper()

	h.submit(t, "/login", "/login", map[string]string{
		"email":    user.Email,
		"password": factories.Password,
	}).AssertRedirect()

	return user
}

// testOrigin is the origin fiber's test transport gives a request, and
// so the origin a browser driving these tests would report.
const testOrigin = "http://example.com"

// visit fetches a page and remembers it as where the browser is, so a
// form posted next carries the Referer a real submission would.
func (h *harness) visit(t *testing.T, path string) *genhttp.TestResponse {
	t.Helper()

	h.page = path
	return h.tc.Get(path)
}

// csrfToken reads the token minted for the current page.
func (h *harness) csrfToken(t *testing.T) string {
	t.Helper()

	if h.page == "" {
		h.visit(t, "/posts").AssertOK()
	}

	token := h.tc.Cookie(middleware.DefaultCSRFConfig().CookieName)
	require.NotEmpty(t, token, "a safe request should mint a CSRF token")
	return token
}

// form posts a form from wherever the browser currently is, with the
// CSRF token attached the way a rendered form carries it.
func (h *harness) form(t *testing.T, action string, fields map[string]string) *genhttp.TestResponse {
	t.Helper()

	values := make(map[string]string, len(fields)+1)
	for key, value := range fields {
		values[key] = value
	}
	values[middleware.DefaultCSRFConfig().FieldName] = h.csrfToken(t)

	request := genhttp.Post(action).WithForm(values)
	if h.page != "" {
		// Absolute, as the spec requires and as browsers send it: the
		// CSRF middleware compares the Referer's host with the request's.
		request.WithHeader("Referer", testOrigin+h.page)
	}

	return h.tc.Do(request)
}

// formRequest builds a form submission with a method other than POST -
// the PUT and DELETE a REST route wants, which a browser reaches through
// a method-override field but a test can send directly.
func (h *harness) formRequest(t *testing.T, method, action string, fields map[string]string) *genhttp.TestRequest {
	t.Helper()

	values := make(map[string]string, len(fields)+1)
	for key, value := range fields {
		values[key] = value
	}
	values[middleware.DefaultCSRFConfig().FieldName] = h.csrfToken(t)

	request := genhttp.NewTestRequest(method, action).WithForm(values)
	if h.page != "" {
		request.WithHeader("Referer", testOrigin+h.page)
	}
	return request
}

// editForm opens a post's edit page and saves it, which is the gesture
// the uniqueness-ignoring rule has to survive.
func (h *harness) editForm(t *testing.T, post *models.Post, fields map[string]string) *genhttp.TestResponse {
	t.Helper()

	h.visit(t, "/posts/"+post.Slug+"/edit").AssertOK()
	return h.tc.Do(h.formRequest(t, "PUT", "/posts/"+post.Slug, fields))
}

// submit is the whole browser gesture: open the page carrying the form,
// then post it. A validation failure comes back to that page, which is
// what makes "back with errors and old input" assertable.
func (h *harness) submit(t *testing.T, page, action string, fields map[string]string) *genhttp.TestResponse {
	t.Helper()

	h.visit(t, page).AssertOK()
	return h.form(t, action, fields)
}
