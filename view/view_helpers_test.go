package view_test

import (
	"strings"
	"testing"

	"github.com/genesysflow/go-genesys/view"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A template using the helpers must parse even when nothing is wired up:
// an unregistered function is a parse error that takes down every view.
func TestHelpersParseWithoutWiring(t *testing.T) {
	root := t.TempDir()
	writeView(t, root, "page.html",
		`{{route "users.show"}}|{{url "/about"}}|{{asset "css/app.css"}}|{{config "app.name"}}|{{trans "messages.hi"}}`)

	m := view.NewManager(view.Config{Path: root})
	out, err := m.RenderString("page", nil)
	require.NoError(t, err)

	// Unwired helpers degrade to something harmless rather than blowing up.
	assert.Equal(t, "|/about|/css/app.css||messages.hi", out)
}

func TestRouteHelper(t *testing.T) {
	root := t.TempDir()
	writeView(t, root, "page.html", `<a href="{{route "users.show" (dict "user" 42)}}">show</a>`)

	m := view.NewManager(view.Config{Path: root})
	m.SetURLGenerator(func(name string, params ...map[string]any) string {
		if len(params) > 0 {
			return "/users/42"
		}
		return "/users/" + name
	})

	out, err := m.RenderString("page", nil)
	require.NoError(t, err)
	assert.Equal(t, `<a href="/users/42">show</a>`, out)
}

func TestURLAndAssetHelpersUseBaseURL(t *testing.T) {
	root := t.TempDir()
	writeView(t, root, "page.html", `{{url "/about"}}|{{asset "css/app.css"}}`)

	m := view.NewManager(view.Config{Path: root})
	m.SetBaseURL("https://example.com/")

	out, err := m.RenderString("page", nil)
	require.NoError(t, err)
	assert.Equal(t, "https://example.com/about|https://example.com/css/app.css", out)
}

func TestConfigAndTransHelpers(t *testing.T) {
	root := t.TempDir()
	writeView(t, root, "page.html", `{{config "app.name"}} says {{trans "messages.hi"}}`)

	m := view.NewManager(view.Config{Path: root})
	m.SetConfigResolver(func(key string) any {
		return map[string]any{"app.name": "Genesys"}[key]
	})
	m.SetTranslator(func(key string, replacements ...map[string]string) string {
		return map[string]string{"messages.hi": "hello"}[key]
	})

	out, err := m.RenderString("page", nil)
	require.NoError(t, err)
	assert.Equal(t, "Genesys says hello", out)
}

func TestCSRFAndMethodFields(t *testing.T) {
	root := t.TempDir()
	writeView(t, root, "form.html", `<form>{{csrf_field .csrf_token}}{{method_field "PUT"}}</form>`)

	m := view.NewManager(view.Config{Path: root})
	out, err := m.RenderString("form", map[string]any{"csrf_token": "abc123"})
	require.NoError(t, err)

	assert.Contains(t, out, `<input type="hidden" name="_token" value="abc123">`)
	assert.Contains(t, out, `<input type="hidden" name="_method" value="PUT">`)
}

// A token is attacker-influenced in the sense that it ends up inside an
// attribute; it must be escaped, not interpolated raw.
func TestCSRFFieldEscapesToken(t *testing.T) {
	root := t.TempDir()
	writeView(t, root, "form.html", `{{csrf_field .csrf_token}}`)

	m := view.NewManager(view.Config{Path: root})
	out, err := m.RenderString("form", map[string]any{"csrf_token": `"><script>alert(1)</script>`})
	require.NoError(t, err)

	assert.NotContains(t, out, "<script>")
	assert.Contains(t, out, "&lt;script&gt;")
}

// Only real HTTP verbs are spoofable; anything else renders nothing
// rather than smuggling markup into the form.
func TestMethodFieldRejectsNonVerbs(t *testing.T) {
	root := t.TempDir()
	writeView(t, root, "form.html", `{{method_field .method}}`)

	m := view.NewManager(view.Config{Path: root})
	out, err := m.RenderString("form", map[string]any{"method": `"><script>alert(1)</script>`})
	require.NoError(t, err)
	assert.Equal(t, "", out)

	out, err = m.RenderString("form", map[string]any{"method": "delete"})
	require.NoError(t, err)
	assert.Contains(t, out, `value="DELETE"`)
}

// --- composers -------------------------------------------------------

func TestComposerRunsForMatchingView(t *testing.T) {
	root := t.TempDir()
	writeView(t, root, "users/index.html", "{{.count}} users")
	writeView(t, root, "posts/index.html", "{{.count}} posts")

	m := view.NewManager(view.Config{Path: root})
	m.Composer("users.index", func(data map[string]any) {
		data["count"] = 3
	})

	out, err := m.RenderString("users.index", nil)
	require.NoError(t, err)
	assert.Equal(t, "3 users", out)

	out, err = m.RenderString("posts.index", nil)
	require.NoError(t, err)
	assert.Equal(t, " posts", out)
}

func TestComposerWildcard(t *testing.T) {
	root := t.TempDir()
	writeView(t, root, "users/index.html", "{{.nav}}")
	writeView(t, root, "users/show.html", "{{.nav}}")
	writeView(t, root, "home.html", "{{.nav}}")

	m := view.NewManager(view.Config{Path: root})
	m.Composer("users.*", func(data map[string]any) {
		data["nav"] = "users-nav"
	})

	for _, name := range []string{"users.index", "users.show"} {
		out, err := m.RenderString(name, nil)
		require.NoError(t, err)
		assert.Equal(t, "users-nav", out, name)
	}

	out, err := m.RenderString("home", nil)
	require.NoError(t, err)
	assert.Equal(t, "", out)
}

// Handler data wins over a composer, which only fills in defaults.
func TestComposerDoesNotOverrideHandlerData(t *testing.T) {
	root := t.TempDir()
	writeView(t, root, "page.html", "{{.title}}")

	m := view.NewManager(view.Config{Path: root})
	m.Composer("*", func(data map[string]any) {
		data["title"] = "from composer"
	})

	out, err := m.RenderString("page", map[string]any{"title": "from handler"})
	require.NoError(t, err)
	assert.Equal(t, "from handler", out)
}

// Composers run in registration order, and every matching one runs.
func TestComposersRunInOrder(t *testing.T) {
	root := t.TempDir()
	writeView(t, root, "page.html", "{{.trail}}")

	m := view.NewManager(view.Config{Path: root})
	m.Composer("*", func(data map[string]any) {
		data["trail"] = "first"
	})
	m.Composer("page", func(data map[string]any) {
		data["trail"] = data["trail"].(string) + "-second"
	})

	out, err := m.RenderString("page", nil)
	require.NoError(t, err)
	assert.Equal(t, "first-second", out)
}

// A composer must not leak values between renders through a shared map.
func TestComposerDataIsPerRender(t *testing.T) {
	root := t.TempDir()
	writeView(t, root, "page.html", "{{.n}}")

	m := view.NewManager(view.Config{Path: root})
	calls := 0
	m.Composer("page", func(data map[string]any) {
		calls++
		data["n"] = calls
	})

	first, err := m.RenderString("page", nil)
	require.NoError(t, err)
	second, err := m.RenderString("page", nil)
	require.NoError(t, err)

	assert.Equal(t, "1", first)
	assert.Equal(t, "2", second)
	assert.False(t, strings.Contains(second, "1"))
}
