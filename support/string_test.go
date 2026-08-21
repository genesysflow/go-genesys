package support_test

import (
	"errors"
	"testing"
	"time"

	"github.com/genesysflow/go-genesys/support"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToPascalCase(t *testing.T) {
	cases := map[string]string{
		"send_email":         "SendEmail",
		"send-email":         "SendEmail",
		"send email":         "SendEmail",
		"sendEmail":          "SendEmail",
		"SendEmail":          "SendEmail",
		"CREATE_USERS_TABLE": "CreateUsersTable",
		"user":               "User",
		"blogPost":           "BlogPost",
	}
	for input, expected := range cases {
		assert.Equal(t, expected, support.ToPascalCase(input), "input %q", input)
	}
}

func TestTitle(t *testing.T) {
	cases := map[string]string{
		"hello world":      "Hello World",
		"hello WORLD":      "Hello World",
		"user name":        "User Name",
		"":                 "",
		"a":                "A",
		"  spaced  words ": "  Spaced  Words ",
		"café au lait":     "Café Au Lait",
		"x1y sees 2 words": "X1y Sees 2 Words",
	}
	for input, expected := range cases {
		assert.Equal(t, expected, support.Title(input), "input %q", input)
	}
}

func TestToSnakeCase(t *testing.T) {
	cases := map[string]string{
		"SendEmailJob": "send_email_job",
		"User":         "user",
		"BlogPost":     "blog_post",
		"already_ok":   "already_ok",
	}
	for input, expected := range cases {
		assert.Equal(t, expected, support.ToSnakeCase(input), "input %q", input)
	}
}

func TestStringHelper(t *testing.T) {
	s := &support.StringHelper{}

	assert.Equal(t, "hello-world", s.Slug("Hello World!"))
	assert.Equal(t, "helloWorld", s.Camel("hello_world"))
	assert.Equal(t, "HelloWorld", s.Pascal("hello_world"))
	assert.Equal(t, "hello_world", s.Snake("HelloWorld"))
	assert.Equal(t, "hello-world", s.Kebab("HelloWorld"))
	assert.Equal(t, "HELLO", s.Upper("hello"))
	assert.Equal(t, "hello", s.Lower("HELLO"))
	assert.Equal(t, "hell...", s.Limit("hello world", 4))
	assert.Equal(t, "hell!", s.Limit("hello world", 4, "!"))
	assert.Equal(t, "hello", s.Limit("hello", 10))
	assert.True(t, s.Contains("hello", "ell"))
	assert.True(t, s.StartsWith("hello", "he"))
	assert.True(t, s.EndsWith("hello", "lo"))
	assert.Equal(t, "hi", s.Trim("  hi  "))
	assert.Equal(t, "hallo", s.Replace("hello", "e", "a"))

	assert.Len(t, s.Random(16), 16)
	assert.NotEqual(t, s.Random(16), s.Random(16))
	assert.Len(t, s.UUID(), 36)
}

func TestArrayAndPathHelpers(t *testing.T) {
	a := &support.ArrayHelper{}
	assert.True(t, a.Contains([]string{"a", "b"}, "b"))
	assert.False(t, a.Contains([]string{"a", "b"}, "z"))
	assert.Equal(t, "a", a.First([]string{"a", "b"}))
	assert.Equal(t, "b", a.Last([]string{"a", "b"}))

	p := &support.PathHelper{}
	assert.Equal(t, "file.txt", p.Base("/tmp/file.txt"))
	assert.Equal(t, ".txt", p.Ext("/tmp/file.txt"))
	assert.True(t, p.Exists(t.TempDir()))
	assert.True(t, p.IsDir(t.TempDir()))
	assert.False(t, p.IsFile(t.TempDir()))
}

func TestFunctionalHelpers(t *testing.T) {
	tapped := 0
	result := support.Tap(41, func(n int) { tapped = n })
	assert.Equal(t, 41, result)
	assert.Equal(t, 41, tapped)

	doubled := support.With(21, func(n int) int { return n * 2 })
	assert.Equal(t, 42, doubled)

	assert.Equal(t, 2, support.When(true, 1, func(n int) int { return n + 1 }))
	assert.Equal(t, 1, support.When(false, 1, func(n int) int { return n + 1 }))
	assert.Equal(t, 2, support.Unless(false, 1, func(n int) int { return n + 1 }))
	assert.Equal(t, 1, support.Unless(true, 1, func(n int) int { return n + 1 }))
}

func TestRetry(t *testing.T) {
	attempts := 0
	err := support.Retry(3, time.Millisecond, func() error {
		attempts++
		if attempts < 3 {
			return errors.New("not yet")
		}
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, 3, attempts)

	attempts = 0
	err = support.Retry(2, time.Millisecond, func() error {
		attempts++
		return errors.New("always")
	})
	assert.Error(t, err)
	assert.Equal(t, 2, attempts)
}
