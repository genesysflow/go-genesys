package collection_test

import (
	"testing"

	"github.com/genesysflow/go-genesys/support/collection"
	"github.com/stretchr/testify/assert"
)

func TestBasics(t *testing.T) {
	c := collection.Collect(1, 2, 3, 4, 5)

	assert.Equal(t, 5, c.Len())
	assert.False(t, c.IsEmpty())
	assert.Equal(t, 1, c.First())
	assert.Equal(t, 5, c.Last())

	evens := c.Filter(func(n int) bool { return n%2 == 0 })
	assert.Equal(t, []int{2, 4}, evens.All())

	odds := c.Reject(func(n int) bool { return n%2 == 0 })
	assert.Equal(t, []int{1, 3, 5}, odds.All())

	doubled := c.Map(func(n int) int { return n * 2 })
	assert.Equal(t, []int{2, 4, 6, 8, 10}, doubled.All())

	assert.True(t, c.Contains(func(n int) bool { return n == 3 }))
	assert.True(t, c.Every(func(n int) bool { return n > 0 }))
	assert.False(t, c.Every(func(n int) bool { return n > 1 }))
}

func TestSlicingAndOrdering(t *testing.T) {
	c := collection.Collect(3, 1, 4, 1, 5, 9)

	assert.Equal(t, []int{3, 1}, c.Take(2).All())
	assert.Equal(t, []int{5, 9}, c.Take(-2).All())
	assert.Equal(t, []int{4, 1, 5, 9}, c.Skip(2).All())
	assert.Equal(t, []int{9, 5, 1, 4, 1, 3}, c.Reverse().All())
	assert.Equal(t, []int{1, 1, 3, 4, 5, 9}, c.SortBy(func(a, b int) bool { return a < b }).All())

	chunks := c.Chunk(4)
	assert.Len(t, chunks, 2)
	assert.Equal(t, []int{3, 1, 4, 1}, chunks[0].All())
	assert.Equal(t, []int{5, 9}, chunks[1].All())
}

func TestTypedHelpers(t *testing.T) {
	type user struct {
		Name string
		Team string
		Age  int
	}
	users := collection.Collect(
		user{"Alice", "eng", 30},
		user{"Bob", "sales", 25},
		user{"Carol", "eng", 41},
	)

	names := collection.Pluck(users, func(u user) string { return u.Name })
	assert.Equal(t, []string{"Alice", "Bob", "Carol"}, names)

	totalAge := collection.Sum(users, func(u user) int { return u.Age })
	assert.Equal(t, 96, totalAge)

	byTeam := collection.GroupBy(users, func(u user) string { return u.Team })
	assert.Len(t, byTeam["eng"], 2)
	assert.Len(t, byTeam["sales"], 1)

	byName := collection.KeyBy(users, func(u user) string { return u.Name })
	assert.Equal(t, 41, byName["Carol"].Age)

	uniqueTeams := collection.Unique(users, func(u user) string { return u.Team })
	assert.Equal(t, 2, uniqueTeams.Len())

	joined := collection.Reduce(users, "", func(carry string, u user) string {
		if carry == "" {
			return u.Name
		}
		return carry + "," + u.Name
	})
	assert.Equal(t, "Alice,Bob,Carol", joined)

	lengths := collection.Map(users, func(u user) int { return len(u.Name) })
	assert.Equal(t, []int{5, 3, 5}, lengths.All())
}
