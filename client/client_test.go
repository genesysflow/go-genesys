package client_test

import (
	"encoding/json"
	"io"
	nethttp "net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/genesysflow/go-genesys/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetWithHeadersAndQuery(t *testing.T) {
	server := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		assert.Equal(t, "Bearer tok-1", r.Header.Get("Authorization"))
		assert.Equal(t, "42", r.URL.Query().Get("page"))
		w.Header().Set("X-Custom", "yes")
		json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	defer server.Close()

	resp, err := client.New().
		WithToken("tok-1").
		WithQuery("page", "42").
		Get(server.URL)
	require.NoError(t, err)

	assert.Equal(t, 200, resp.Status())
	assert.True(t, resp.Successful())
	assert.False(t, resp.Failed())
	assert.Equal(t, "yes", resp.Header("X-Custom"))

	var body map[string]any
	require.NoError(t, resp.JSON(&body))
	assert.Equal(t, true, body["ok"])
}

func TestPostJSONBody(t *testing.T) {
	server := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		raw, _ := io.ReadAll(r.Body)
		assert.JSONEq(t, `{"name":"Alice"}`, string(raw))
		w.WriteHeader(201)
	}))
	defer server.Close()

	resp, err := client.New().Post(server.URL, map[string]string{"name": "Alice"})
	require.NoError(t, err)
	assert.Equal(t, 201, resp.Status())
}

func TestPostFormBody(t *testing.T) {
	server := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		assert.Equal(t, "application/x-www-form-urlencoded", r.Header.Get("Content-Type"))
		require.NoError(t, r.ParseForm())
		assert.Equal(t, "Alice", r.PostForm.Get("name"))
	}))
	defer server.Close()

	resp, err := client.New().AsForm().Post(server.URL, map[string]string{"name": "Alice"})
	require.NoError(t, err)
	assert.True(t, resp.Successful())
}

func TestBaseURLAndBasicAuth(t *testing.T) {
	server := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		user, pass, ok := r.BasicAuth()
		assert.True(t, ok)
		assert.Equal(t, "user", user)
		assert.Equal(t, "pass", pass)
		assert.Equal(t, "/v1/things", r.URL.Path)
	}))
	defer server.Close()

	resp, err := client.New().
		BaseURL(server.URL).
		WithBasicAuth("user", "pass").
		Get("/v1/things")
	require.NoError(t, err)
	assert.True(t, resp.Successful())
}

func TestRetryOnServerError(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		if calls.Add(1) < 3 {
			w.WriteHeader(500)
			return
		}
		w.WriteHeader(200)
	}))
	defer server.Close()

	resp, err := client.New().Retry(3, time.Millisecond).Get(server.URL)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.Status())
	assert.EqualValues(t, 3, calls.Load())
}

func TestClientErrorNotRetried(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		calls.Add(1)
		w.WriteHeader(404)
	}))
	defer server.Close()

	resp, err := client.New().Retry(3, time.Millisecond).Get(server.URL)
	require.NoError(t, err)
	assert.Equal(t, 404, resp.Status())
	assert.True(t, resp.ClientError())
	assert.EqualValues(t, 1, calls.Load(), "4xx responses must not be retried")
}
