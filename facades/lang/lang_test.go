package lang_test

import (
	"testing"

	"github.com/genesysflow/go-genesys/facades/lang"
	baselang "github.com/genesysflow/go-genesys/lang"
	"github.com/stretchr/testify/assert"
)

func TestFacadeTranslation(t *testing.T) {
	translator := baselang.New(baselang.Config{Path: t.TempDir()})
	translator.AddLine("en", "messages.hi", "Hi, :name!")
	translator.AddLine("en", "messages.items", "one item|:count items")
	translator.AddLine("de", "messages.hi", "Hallo, :name!")
	lang.SetInstance(translator)
	t.Cleanup(func() { lang.SetInstance(nil) })

	assert.Equal(t, "Hi, Ada!", lang.Trans("messages.hi", map[string]string{"name": "Ada"}))
	assert.Equal(t, "Hi, Ada!", lang.T("messages.hi", map[string]string{"name": "Ada"}))
	assert.Equal(t, "3 items", lang.TransChoice("messages.items", 3))

	lang.SetLocale("de")
	assert.Equal(t, "de", lang.Locale())
	assert.Equal(t, "Hallo, Ada!", lang.Trans("messages.hi", map[string]string{"name": "Ada"}))

	assert.NotNil(t, lang.GetInstance())
}

func TestFacadePanicsWithoutInstance(t *testing.T) {
	lang.SetInstance(nil)
	assert.Panics(t, func() { lang.Trans("x") })
}
