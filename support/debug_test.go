package support

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDataGet(t *testing.T) {
	payload := map[string]any{
		"user": map[string]any{
			"name":    "Ada",
			"address": map[string]any{"city": "London"},
		},
		"orders": []any{
			map[string]any{"total": 10},
			map[string]any{"total": 25},
		},
	}

	assert.Equal(t, "Ada", DataGet(payload, "user.name"))
	assert.Equal(t, "London", DataGet(payload, "user.address.city"))
	assert.Equal(t, 25, DataGet(payload, "orders.1.total"))
	assert.Equal(t, []any{10, 25}, DataGet(payload, "orders.*.total"))

	assert.Nil(t, DataGet(payload, "user.missing"))
	assert.Equal(t, "fallback", DataGet(payload, "user.missing", "fallback"))
	assert.Equal(t, 0, DataGet(payload, "orders.9.total", 0))
}

func TestDataGetOnStructsAndTypedSlices(t *testing.T) {
	type Address struct{ City string }
	type Person struct {
		Name    string
		Address *Address
		Tags    []string
	}

	person := Person{Name: "Grace", Address: &Address{City: "NYC"}, Tags: []string{"a", "b"}}

	assert.Equal(t, "Grace", DataGet(person, "Name"))
	assert.Equal(t, "NYC", DataGet(&person, "Address.City"))
	assert.Equal(t, "b", DataGet(person, "Tags.1"))
	assert.Equal(t, []any{"a", "b"}, DataGet(person, "Tags.*"))
	assert.Nil(t, DataGet(Person{}, "Address.City"), "nil pointer short-circuits")
}

func TestDumpAndDd(t *testing.T) {
	var out strings.Builder
	oldWriter, oldExit := dumpWriter, exit
	defer func() { dumpWriter, exit = oldWriter, oldExit }()
	dumpWriter = &out

	exitCode := -1
	exit = func(code int) { exitCode = code }

	Dump(map[string]any{"k": "v"})
	assert.Contains(t, out.String(), `"k": "v"`)

	Dd("stop here")
	assert.Contains(t, out.String(), "stop here")
	assert.Equal(t, 1, exitCode)
}
