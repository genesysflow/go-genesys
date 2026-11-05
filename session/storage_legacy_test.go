package session_test

import (
	"bytes"
	"encoding/gob"
	"io"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/genesysflow/go-genesys/session"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A session written by Fiber v2 is a gob map[string]any, which v3 cannot
// decode. It must still load after the upgrade, not fail every request
// that carries its cookie.
func TestSessionWrittenByFiberV2StillLoads(t *testing.T) {
	for name, storage := range storages(t) {
		t.Run(name, func(t *testing.T) {
			var payload bytes.Buffer
			legacy := map[string]any{
				"user_id": 42,
				"_flash":  map[string]any{"status": "Saved."},
			}
			require.NoError(t, gob.NewEncoder(&payload).Encode(&legacy))
			require.NoError(t, storage.Set("v2-session", payload.Bytes(), time.Hour))

			manager := session.NewManager(session.Config{
				CookieName:    "sid",
				CustomStorage: storage,
			})
			app := fiber.New()
			app.Use(manager.Middleware())
			app.Get("/", func(c fiber.Ctx) error {
				sess := session.GetFromContext(c)
				assert.Equal(t, "v2-session", sess.ID())
				assert.Equal(t, "Saved.", sess.GetString("status"))
				return c.SendString(strconv.Itoa(sess.GetInt("user_id")))
			})

			req := httptest.NewRequest("GET", "/", nil)
			req.Header.Set("Cookie", "sid=v2-session")
			resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
			require.NoError(t, err)
			body, _ := io.ReadAll(resp.Body)
			assert.Equal(t, 200, resp.StatusCode)
			assert.Equal(t, "42", string(body))
		})
	}
}
