package support_test

import (
	"testing"

	"github.com/genesysflow/go-genesys/support"
	"github.com/stretchr/testify/assert"
)

func TestMessageBagEmpty(t *testing.T) {
	bag := support.NewMessageBag(nil)

	assert.False(t, bag.Any())
	assert.Equal(t, 0, bag.Count())
	assert.False(t, bag.Has("email"))
	assert.Equal(t, "", bag.First("email"))
	assert.Empty(t, bag.Get("email"))
	assert.Empty(t, bag.All())
	assert.Empty(t, bag.Keys())
}

func TestMessageBagAccessors(t *testing.T) {
	bag := support.NewMessageBag(map[string][]string{
		"email": {"The email field is required.", "The email must be valid."},
		"name":  {"The name field is required."},
	})

	assert.True(t, bag.Any())
	assert.Equal(t, 3, bag.Count())
	assert.True(t, bag.Has("email"))
	assert.False(t, bag.Has("missing"))
	assert.Equal(t, "The email field is required.", bag.First("email"))
	assert.Equal(t, "", bag.First("missing"))
	assert.Len(t, bag.Get("email"), 2)
	assert.Len(t, bag.All(), 3)
	assert.ElementsMatch(t, []string{"email", "name"}, bag.Keys())
}

// First() with no field returns the first message in the bag, matching
// Laravel's $errors->first().
func TestMessageBagFirstWithoutField(t *testing.T) {
	bag := support.NewMessageBag(map[string][]string{"email": {"Required."}})
	assert.Equal(t, "Required.", bag.First())

	assert.Equal(t, "", support.NewMessageBag(nil).First())
}

func TestMessageBagAdd(t *testing.T) {
	bag := support.NewMessageBag(nil)
	bag.Add("email", "Required.")
	bag.Add("email", "Invalid.")

	assert.True(t, bag.Has("email"))
	assert.Equal(t, []string{"Required.", "Invalid."}, bag.Get("email"))
	assert.Equal(t, 2, bag.Count())
}

// Messages() hands back a copy: mutating it must not corrupt the bag.
func TestMessageBagMessagesIsACopy(t *testing.T) {
	bag := support.NewMessageBag(map[string][]string{"email": {"Required."}})

	messages := bag.Messages()
	messages["email"] = append(messages["email"], "Injected.")
	delete(messages, "name")

	assert.Equal(t, []string{"Required."}, bag.Get("email"))
}

// The source map is copied on construction too.
func TestMessageBagCopiesSource(t *testing.T) {
	source := map[string][]string{"email": {"Required."}}
	bag := support.NewMessageBag(source)

	source["email"] = append(source["email"], "Injected.")

	assert.Equal(t, []string{"Required."}, bag.Get("email"))
}
