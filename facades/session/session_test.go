package session_test

import (
	"net/http/httptest"
	"testing"

	facadesession "github.com/genesysflow/go-genesys/facades/session"
	basesession "github.com/genesysflow/go-genesys/session"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFacadeMiddlewareAndFromContext(t *testing.T) {
	facadesession.SetInstance(basesession.NewManager(basesession.Config{CookieSecure: false}))
	t.Cleanup(func() { facadesession.SetInstance(nil) })

	app := fiber.New()
	app.Use(facadesession.Middleware())
	app.Get("/", func(c *fiber.Ctx) error {
		sess := facadesession.FromContext(c)
		require.NotNil(t, sess)
		sess.Set("k", "v")
		return c.SendString(sess.GetString("k"))
	})

	resp, err := app.Test(httptest.NewRequest("GET", "/", nil), -1)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.NotNil(t, facadesession.GetInstance())
}

func TestFromContextWithoutMiddleware(t *testing.T) {
	app := fiber.New()
	app.Get("/", func(c *fiber.Ctx) error {
		assert.Nil(t, facadesession.FromContext(c))
		return c.SendString("ok")
	})
	_, err := app.Test(httptest.NewRequest("GET", "/", nil), -1)
	require.NoError(t, err)
}

func TestFacadePanicsWithoutInstance(t *testing.T) {
	facadesession.SetInstance(nil)
	assert.Panics(t, func() { facadesession.Middleware() })
}
