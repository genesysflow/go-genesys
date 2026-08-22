package http_test

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/genesysflow/go-genesys/database"
	"github.com/genesysflow/go-genesys/foundation"
	genhttp "github.com/genesysflow/go-genesys/http"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// account stands in for an application's user model: an id and a
// privilege column the request has no business setting.
type account struct {
	database.Model
	Email string `db:"email" json:"email" form:"email"`
	Role  string `db:"role" json:"role" form:"role"`
}

// Binding a request body straight into a model is the shortest path a
// developer can take, so it must not be the one that hands the client
// the id and the role columns.
func TestBindRefusesToFillAModelWholesale(t *testing.T) {
	app := foundation.New()
	require.NoError(t, app.Boot())

	kernel := genhttp.NewKernel(app, genhttp.KernelConfig{DisableStartupMessage: true})

	var bound account
	kernel.POST("/accounts", func(ctx *genhttp.Context) error {
		if err := ctx.Bind(&bound); err != nil {
			return err
		}
		return ctx.String("ok")
	})

	req := httptest.NewRequest("POST", "/accounts",
		strings.NewReader(`{"email":"ada@example.com","role":"admin","id":99}`))
	req.Header.Set("Content-Type", "application/json")

	_, err := kernel.Fiber().Test(req, -1)
	require.NoError(t, err)

	assert.Equal(t, "ada@example.com", bound.Email, "declared fields still bind")
	assert.Equal(t, int64(0), bound.ID,
		"a client must not be able to choose a model's primary key")
}
