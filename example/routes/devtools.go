package routes

import (
	"github.com/genesysflow/go-genesys/container"
	"github.com/genesysflow/go-genesys/database"
	"github.com/genesysflow/go-genesys/devtools"
	"github.com/genesysflow/go-genesys/http"
)

// recorder collects what the application does, for the dev panel.
var recorder = devtools.NewRecorder(200)

// Recorder exposes the recorder so tests can read what was recorded.
func Recorder() *devtools.Recorder { return recorder }

// Devtools mounts the development panel and starts recording. In
// production Register refuses to mount, and nothing is recorded.
func Devtools(r *http.Router) {
	app := r.App()
	if app == nil || app.IsProduction() {
		return
	}

	r.Use(devtools.Middleware(recorder))

	if manager, err := container.Resolve[*database.Manager](app); err == nil {
		manager.Listen(recorder.RecordQuery)
	}

	// Mounting refuses in production, which is the point: the panel
	// serves request paths and SQL.
	_ = devtools.RegisterRoutes(r, recorder, "/_genesys")
}
