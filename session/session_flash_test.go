package session_test

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/genesysflow/go-genesys/session"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// flashHarness runs handlers through a real Fiber app with the session
// middleware, carrying cookies between requests like a browser would.
type flashHarness struct {
	t       *testing.T
	app     *fiber.App
	cookies string
}

func newFlashHarness(t *testing.T, register func(app *fiber.App, manager *session.Manager)) *flashHarness {
	t.Helper()
	manager := session.NewManager(session.Config{
		CookieName:   "harness_session",
		CookieSecure: false,
	})
	app := fiber.New()
	app.Use(manager.Middleware())
	register(app, manager)
	return &flashHarness{t: t, app: app}
}

func (h *flashHarness) get(path string) string {
	h.t.Helper()
	req := httptest.NewRequest("GET", path, nil)
	if h.cookies != "" {
		req.Header.Set("Cookie", h.cookies)
	}
	resp, err := h.app.Test(req, -1)
	require.NoError(h.t, err)

	var parts []string
	for _, c := range resp.Cookies() {
		parts = append(parts, c.Name+"="+c.Value)
	}
	if len(parts) > 0 {
		h.cookies = strings.Join(parts, "; ")
	}

	body := make([]byte, 4096)
	n, _ := resp.Body.Read(body)
	return string(body[:n])
}

func TestFlashLivesExactlyOneRequest(t *testing.T) {
	h := newFlashHarness(t, func(app *fiber.App, _ *session.Manager) {
		app.Get("/flash", func(c *fiber.Ctx) error {
			sess := session.GetFromContext(c)
			require.NoError(t, sess.Flash("status", "saved!"))
			// Flash data is readable in the same request.
			return c.SendString(sess.GetString("status"))
		})
		app.Get("/read", func(c *fiber.Ctx) error {
			return c.SendString(session.GetFromContext(c).GetString("status"))
		})
	})

	assert.Equal(t, "saved!", h.get("/flash"), "flash readable within the flashing request")
	assert.Equal(t, "saved!", h.get("/read"), "flash readable on the next request")
	assert.Equal(t, "", h.get("/read"), "flash gone on the request after that")
}

func TestReflashAndKeep(t *testing.T) {
	h := newFlashHarness(t, func(app *fiber.App, _ *session.Manager) {
		app.Get("/flash", func(c *fiber.Ctx) error {
			sess := session.GetFromContext(c)
			sess.Flash("a", "1")
			sess.Flash("b", "2")
			return c.SendString("ok")
		})
		app.Get("/keep-a", func(c *fiber.Ctx) error {
			sess := session.GetFromContext(c)
			require.NoError(t, sess.Keep("a"))
			return c.SendString("ok")
		})
		app.Get("/reflash", func(c *fiber.Ctx) error {
			sess := session.GetFromContext(c)
			require.NoError(t, sess.Reflash())
			return c.SendString("ok")
		})
		app.Get("/read", func(c *fiber.Ctx) error {
			sess := session.GetFromContext(c)
			return c.SendString(sess.GetString("a") + "|" + sess.GetString("b"))
		})
	})

	h.get("/flash")
	h.get("/keep-a")
	assert.Equal(t, "1|", h.get("/read"), "only kept keys survive an extra request")

	h.get("/flash")
	h.get("/reflash")
	assert.Equal(t, "1|2", h.get("/read"), "reflash extends all flash data")
}

func TestOldInputHelpers(t *testing.T) {
	h := newFlashHarness(t, func(app *fiber.App, _ *session.Manager) {
		app.Get("/submit", func(c *fiber.Ctx) error {
			sess := session.GetFromContext(c)
			require.NoError(t, sess.FlashInput(map[string]any{"email": "a@x.io", "name": "Ada"}))
			return c.SendString("ok")
		})
		app.Get("/form", func(c *fiber.Ctx) error {
			sess := session.GetFromContext(c)
			if !sess.HasOldInput() {
				return c.SendString("none")
			}
			return c.SendString(sess.Old("email") + "|" + sess.Old("missing", "fallback"))
		})
	})

	assert.Equal(t, "none", h.get("/form"))
	h.get("/submit")
	assert.Equal(t, "a@x.io|fallback", h.get("/form"))
	assert.Equal(t, "none", h.get("/form"), "old input ages out with the flash")
}

func TestPersistentDataAndHelpers(t *testing.T) {
	h := newFlashHarness(t, func(app *fiber.App, _ *session.Manager) {
		app.Get("/set", func(c *fiber.Ctx) error {
			sess := session.GetFromContext(c)
			sess.Set("theme", "dark")
			sess.Set("count", 3)
			sess.Set("admin", true)
			return c.SendString(sess.ID())
		})
		app.Get("/read", func(c *fiber.Ctx) error {
			sess := session.GetFromContext(c)
			if !sess.Has("theme") {
				return c.SendString("missing")
			}
			out := sess.GetString("theme")
			if sess.GetInt("count") == 3 && sess.GetBool("admin") {
				out += "|typed"
			}
			return c.SendString(out)
		})
		app.Get("/pull", func(c *fiber.Ctx) error {
			sess := session.GetFromContext(c)
			value, _ := sess.Pull("theme").(string)
			return c.SendString(value)
		})
		app.Get("/flush", func(c *fiber.Ctx) error {
			sess := session.GetFromContext(c)
			require.NoError(t, sess.Flush())
			return c.SendString("ok")
		})
	})

	id := h.get("/set")
	assert.NotEmpty(t, id)
	assert.Equal(t, "dark|typed", h.get("/read"), "persistent data survives requests")
	assert.Equal(t, "dark", h.get("/pull"))
	assert.Equal(t, "missing", h.get("/read"), "Pull removed the key")

	h.get("/set")
	h.get("/flush")
	assert.Equal(t, "missing", h.get("/read"), "Flush cleared the session")
}

// Presence is asked by key, not inferred by comparing against a
// sentinel: a flashed value that happens to equal the sentinel would
// otherwise report absent.
func TestHasOldAnswersByKey(t *testing.T) {
	h := newFlashHarness(t, func(app *fiber.App, _ *session.Manager) {
		app.Get("/flash", func(c *fiber.Ctx) error {
			sess := session.GetFromContext(c)
			sess.FlashInput(map[string]any{
				"email":    "ada@example.com",
				"empty":    "",
				"sentinel": "\x00missing",
			})
			return c.SendString("flashed")
		})
		app.Get("/read", func(c *fiber.Ctx) error {
			sess := session.GetFromContext(c)
			return c.SendString(fmt.Sprintf("%v|%v|%v|%v",
				sess.HasOld("email"),
				sess.HasOld("empty"),
				sess.HasOld("sentinel"),
				sess.HasOld("never-flashed")))
		})
	})

	h.get("/flash")

	// A flashed empty string is still input, and any value is a value.
	assert.Equal(t, "true|true|true|false", h.get("/read"))
}
