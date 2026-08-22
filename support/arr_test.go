package support_test

import (
	"testing"

	"github.com/genesysflow/go-genesys/support"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArrMapFilterReduce(t *testing.T) {
	numbers := []int{1, 2, 3, 4}

	assert.Equal(t, []string{"1", "2", "3", "4"}, support.MapSlice(numbers, func(n int) string {
		return string(rune('0' + n))
	}))
	assert.Equal(t, []int{2, 4}, support.Filter(numbers, func(n int) bool { return n%2 == 0 }))
	assert.Equal(t, 10, support.Reduce(numbers, 0, func(carry, n int) int { return carry + n }))
}

func TestArrUniqueFlattenChunk(t *testing.T) {
	assert.Equal(t, []int{1, 2, 3}, support.Unique([]int{1, 2, 2, 3, 1}))
	assert.Equal(t, []int{1, 2, 3, 4}, support.Flatten([][]int{{1, 2}, {3, 4}}))

	chunks := support.Chunk([]int{1, 2, 3, 4, 5}, 2)
	require.Len(t, chunks, 3)
	assert.Equal(t, []int{5}, chunks[2])

	// A chunk size of zero would loop forever; it yields nothing instead.
	assert.Empty(t, support.Chunk([]int{1, 2}, 0))
}

func TestArrKeyByAndGroupBy(t *testing.T) {
	type user struct {
		ID   int
		Team string
	}
	users := []user{{1, "red"}, {2, "blue"}, {3, "red"}}

	byID := support.KeyBy(users, func(u user) int { return u.ID })
	assert.Equal(t, "blue", byID[2].Team)

	byTeam := support.GroupBy(users, func(u user) string { return u.Team })
	assert.Len(t, byTeam["red"], 2)
}

func TestArrOnlyExceptPluck(t *testing.T) {
	data := map[string]any{"name": "Ada", "email": "ada@example.com", "password": "secret"}

	assert.Equal(t, map[string]any{"name": "Ada"}, support.Only(data, "name"))

	without := support.Except(data, "password")
	assert.Len(t, without, 2)
	_, present := without["password"]
	assert.False(t, present)

	type user struct{ Name string }
	assert.Equal(t, []string{"Ada", "Grace"}, support.Pluck([]user{{"Ada"}, {"Grace"}}, func(u user) string { return u.Name }))
}

// Only and Except must copy, so the caller's map is untouched.
func TestArrOnlyExceptDoNotMutate(t *testing.T) {
	data := map[string]any{"name": "Ada", "password": "secret"}

	support.Only(data, "name")
	support.Except(data, "password")

	assert.Len(t, data, 2)
}

func TestArrFirstLast(t *testing.T) {
	numbers := []int{1, 2, 3}

	first, ok := support.First(numbers, func(n int) bool { return n > 1 })
	require.True(t, ok)
	assert.Equal(t, 2, first)

	last, ok := support.Last(numbers, func(n int) bool { return n < 3 })
	require.True(t, ok)
	assert.Equal(t, 2, last)

	_, ok = support.First(numbers, func(n int) bool { return n > 99 })
	assert.False(t, ok)
}

func TestArrPartitionAndContains(t *testing.T) {
	even, odd := support.Partition([]int{1, 2, 3, 4}, func(n int) bool { return n%2 == 0 })
	assert.Equal(t, []int{2, 4}, even)
	assert.Equal(t, []int{1, 3}, odd)

	assert.True(t, support.Includes([]string{"a", "b"}, "b"))
	assert.False(t, support.Includes([]string{"a", "b"}, "c"))
}

// --- numbers ---------------------------------------------------------

func TestNumberFormat(t *testing.T) {
	assert.Equal(t, "1,234", support.Num.Format(1234))
	assert.Equal(t, "1,234,567", support.Num.Format(1234567))
	assert.Equal(t, "-1,234", support.Num.Format(-1234))
	assert.Equal(t, "999", support.Num.Format(999))
}

func TestNumberCurrencyAndPercentage(t *testing.T) {
	assert.Equal(t, "$1,234.56", support.Num.Currency(1234.56, "$"))
	assert.Equal(t, "€0.50", support.Num.Currency(0.5, "€"))
	assert.Equal(t, "12.5%", support.Num.Percentage(12.5))
	assert.Equal(t, "13%", support.Num.Percentage(12.5, 0))
}

func TestNumberFileSize(t *testing.T) {
	assert.Equal(t, "512 B", support.Num.FileSize(512))
	assert.Equal(t, "1.0 KB", support.Num.FileSize(1024))
	assert.Equal(t, "1.5 MB", support.Num.FileSize(1024*1024*3/2))
	assert.Equal(t, "2.0 GB", support.Num.FileSize(2*1024*1024*1024))
}

func TestNumberOrdinal(t *testing.T) {
	assert.Equal(t, "1st", support.Num.Ordinal(1))
	assert.Equal(t, "2nd", support.Num.Ordinal(2))
	assert.Equal(t, "3rd", support.Num.Ordinal(3))
	assert.Equal(t, "4th", support.Num.Ordinal(4))
	// The teens are the case a naive implementation gets wrong.
	assert.Equal(t, "11th", support.Num.Ordinal(11))
	assert.Equal(t, "12th", support.Num.Ordinal(12))
	assert.Equal(t, "13th", support.Num.Ordinal(13))
	assert.Equal(t, "21st", support.Num.Ordinal(21))
	assert.Equal(t, "111th", support.Num.Ordinal(111))
}

func TestNumberClamp(t *testing.T) {
	assert.Equal(t, 5, support.Clamp(10, 0, 5))
	assert.Equal(t, 0, support.Clamp(-1, 0, 5))
	assert.Equal(t, 3, support.Clamp(3, 0, 5))
}

// --- pipeline --------------------------------------------------------

func TestPipeline(t *testing.T) {
	result, err := support.Pipe("hello").
		Through(
			func(s string) (string, error) { return s + " world", nil },
			func(s string) (string, error) { return support.Str.Upper(s), nil },
		).
		Run()

	require.NoError(t, err)
	assert.Equal(t, "HELLO WORLD", result)
}

// A stage that fails stops the pipeline: the stages after it never see
// a value the failing stage did not produce.
func TestPipelineStopsOnError(t *testing.T) {
	reached := false

	_, err := support.Pipe(1).
		Through(
			func(n int) (int, error) { return 0, assertPipelineErr },
			func(n int) (int, error) { reached = true; return n, nil },
		).
		Run()

	require.Error(t, err)
	assert.False(t, reached, "a stage after a failure must not run")
}

var assertPipelineErr = pipelineErr{}

type pipelineErr struct{}

func (pipelineErr) Error() string { return "stage failed" }
