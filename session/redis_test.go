package session_test

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/genesysflow/go-genesys/session"
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newRedisStorage(t *testing.T) (*session.RedisStorage, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { client.Close() })
	return session.NewRedisStorageWithClient(client, ""), mr
}

func TestRedisStorageRoundTrip(t *testing.T) {
	storage, mr := newRedisStorage(t)

	require.NoError(t, storage.Set("abc", []byte("payload"), time.Minute))
	value, err := storage.Get("abc")
	require.NoError(t, err)
	assert.Equal(t, []byte("payload"), value)

	missing, err := storage.Get("nope")
	require.NoError(t, err)
	assert.Nil(t, missing)

	mr.FastForward(2 * time.Minute)
	expired, err := storage.Get("abc")
	require.NoError(t, err)
	assert.Nil(t, expired, "sessions expire natively")

	require.NoError(t, storage.Set("gone", []byte("x"), time.Minute))
	require.NoError(t, storage.Delete("gone"))
	value, err = storage.Get("gone")
	require.NoError(t, err)
	assert.Nil(t, value)
}

func TestRedisStorageReset(t *testing.T) {
	storage, mr := newRedisStorage(t)

	require.NoError(t, storage.Set("a", []byte("1"), time.Minute))
	require.NoError(t, storage.Set("b", []byte("2"), time.Minute))
	require.NoError(t, mr.Set("outside", "keep"))

	require.NoError(t, storage.Reset())

	value, err := storage.Get("a")
	require.NoError(t, err)
	assert.Nil(t, value)
	kept, _ := mr.Get("outside")
	assert.Equal(t, "keep", kept, "keys outside the prefix survive Reset")
}

func TestSessionsBackedByRedis(t *testing.T) {
	storage, _ := newRedisStorage(t)

	manager := session.NewManager(session.Config{
		CookieSecure:  false,
		CustomStorage: storage,
	})

	app := fiber.New()
	app.Use(manager.Middleware())
	app.Get("/set", func(c *fiber.Ctx) error {
		sess := session.GetFromContext(c)
		sess.Set("who", "redis")
		return c.SendString("ok")
	})
	app.Get("/get", func(c *fiber.Ctx) error {
		return c.SendString(session.GetFromContext(c).GetString("who"))
	})

	// First request sets the value; carry the cookie to the second.
	resp, err := app.Test(httptest.NewRequest("GET", "/set", nil), -1)
	require.NoError(t, err)
	var cookies []string
	for _, c := range resp.Cookies() {
		cookies = append(cookies, c.Name+"="+c.Value)
	}
	require.NotEmpty(t, cookies)

	req := httptest.NewRequest("GET", "/get", nil)
	req.Header.Set("Cookie", strings.Join(cookies, "; "))
	resp, err = app.Test(req, -1)
	require.NoError(t, err)
	body := make([]byte, 64)
	n, _ := resp.Body.Read(body)
	assert.Equal(t, "redis", string(body[:n]), "session round-trips through Redis")
}
