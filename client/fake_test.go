package client_test

import (
	"testing"

	"github.com/genesysflow/go-genesys/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFakeStubsAndRecords(t *testing.T) {
	fake := client.NewFake().
		Respond("https://api.example.com/users*", 200, `[{"id":1}]`,
			map[string]string{"Content-Type": "application/json"}).
		Respond("*", 404, "nope")

	resp, err := client.New().WithFake(fake).
		WithHeader("X-Api-Key", "secret").
		Get("https://api.example.com/users?page=1")
	require.NoError(t, err)
	assert.Equal(t, 200, resp.Status())
	assert.Equal(t, `[{"id":1}]`, string(resp.Body()))

	missing, err := client.New().WithFake(fake).Get("https://other.example.com/x")
	require.NoError(t, err)
	assert.Equal(t, 404, missing.Status())

	fake.AssertSent(t, "GET", "https://api.example.com/users*")
	fake.AssertNotSent(t, "POST", "*")
	requests := fake.Requests()
	require.Len(t, requests, 2)
	assert.Equal(t, "secret", requests[0].Header.Get("X-Api-Key"))
}

func TestFakeRecordsBodies(t *testing.T) {
	fake := client.NewFake().Respond("*", 201, `{"ok":true}`)

	_, err := client.New().WithFake(fake).Post("https://api.example.com/orders", map[string]any{"sku": "X1"})
	require.NoError(t, err)

	requests := fake.Requests()
	require.Len(t, requests, 1)
	assert.Equal(t, "POST", requests[0].Method)
	assert.JSONEq(t, `{"sku":"X1"}`, requests[0].Body)
}
