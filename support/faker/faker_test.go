package faker_test

import (
	"strings"
	"testing"
	"time"

	"github.com/genesysflow/go-genesys/support/faker"
	"github.com/stretchr/testify/assert"
)

func TestSeededFakerIsDeterministic(t *testing.T) {
	a := faker.NewSeeded(42)
	b := faker.NewSeeded(42)

	assert.Equal(t, a.Name(), b.Name())
	assert.Equal(t, a.Email(), b.Email())
	assert.Equal(t, a.Sentence(6), b.Sentence(6))
	assert.Equal(t, a.IntBetween(1, 100), b.IntBetween(1, 100))
}

func TestGeneratorShapes(t *testing.T) {
	f := faker.NewSeeded(1)

	assert.Contains(t, f.Name(), " ")
	assert.Contains(t, f.Email(), "@")
	assert.NotContains(t, f.Username(), " ")

	words := f.Words(5)
	assert.Len(t, strings.Fields(words), 5)

	sentence := f.Sentence(4)
	assert.True(t, strings.HasSuffix(sentence, "."))
	assert.Equal(t, strings.ToUpper(sentence[:1]), sentence[:1])

	assert.NotEmpty(t, f.Paragraph(3))
	assert.Contains(t, f.URL(), "https://")
	assert.Contains(t, f.Slug(3), "-")
	assert.Regexp(t, `^\+1-\d{3}-\d{3}-\d{4}$`, f.Phone())

	for i := 0; i < 50; i++ {
		n := f.IntBetween(5, 10)
		assert.GreaterOrEqual(t, n, 5)
		assert.LessOrEqual(t, n, 10)
		x := f.Float(1.5, 2.5)
		assert.GreaterOrEqual(t, x, 1.5)
		assert.Less(t, x, 2.5)
	}
	assert.Equal(t, 7, f.IntBetween(7, 7))

	assert.True(t, f.Past(time.Hour).Before(time.Now()))
	assert.True(t, f.Future(time.Hour).After(time.Now()))

	assert.True(t, f.Bool(1.0))
	assert.False(t, f.Bool(0.0))

	choice := faker.Pick(f, "a", "b", "c")
	assert.Contains(t, []string{"a", "b", "c"}, choice)
}
