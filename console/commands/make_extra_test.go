package commands

import (
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGeneratorTemplatesProduceValidGo renders each generator template and
// parses the output, so a broken template fails in CI rather than at
// `genesys make:*` time.
func TestGeneratorTemplatesProduceValidGo(t *testing.T) {
	templates := []string{
		"model.go.tmpl",
		"job.go.tmpl",
		"event.go.tmpl",
		"listener.go.tmpl",
		"seeder.go.tmpl",
		"request.go.tmpl",
		"command.go.tmpl",
		"policy.go.tmpl",
	}

	for _, name := range templates {
		t.Run(name, func(t *testing.T) {
			content, err := render(name, map[string]string{
				"Name":      "Example",
				"LowerName": "example",
				"TableName": "examples",
			})
			require.NoError(t, err)

			_, err = parser.ParseFile(token.NewFileSet(), "generated.go", content, parser.AllErrors)
			assert.NoError(t, err, "template %s must render valid Go:\n%s", name, content)
		})
	}
}
