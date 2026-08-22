package devtools

import (
	"fmt"
	"html/template"
	"strings"
	"time"

	"github.com/genesysflow/go-genesys/http"
)

// Register mounts the panel on a kernel at the given path (default
// "/_genesys") and marks the recorder so the panel's own visits are not
// recorded.
//
// It refuses to mount in production: the panel serves request paths and
// SQL, which is exactly what must not leave a production box. Guarding
// it here means an application cannot enable it there by accident.
func Register(kernel *http.Kernel, recorder *Recorder, path ...string) error {
	return RegisterRoutes(kernel.Router(), recorder, path...)
}

// RegisterRoutes mounts the panel on a router, which is where an
// application registers its routes. Register is the same thing for a
// kernel.
func RegisterRoutes(router *http.Router, recorder *Recorder, path ...string) error {
	mountAt := "/_genesys"
	if len(path) > 0 && path[0] != "" {
		mountAt = path[0]
	}

	app := router.App()
	if app != nil && app.IsProduction() {
		return fmt.Errorf("devtools: the panel exposes request paths and SQL and will not be mounted in production")
	}

	recorder.setPanelPath(mountAt)

	router.GET(mountAt, func(ctx *http.Context) error {
		return ctx.HTML(recorder.render())
	})
	router.POST(mountAt+"/clear", func(ctx *http.Context) error {
		recorder.Clear()
		return ctx.RedirectTo(mountAt).Send()
	})

	return nil
}

// render builds the panel's HTML.
func (r *Recorder) render() string {
	requests := r.Requests()
	queries := r.Queries()

	var body strings.Builder
	body.WriteString(panelHead)

	body.WriteString(`<h2>Requests</h2>`)
	if len(requests) == 0 {
		body.WriteString(`<p class="empty">Nothing recorded yet.</p>`)
	} else {
		body.WriteString(`<table><tr><th>Method</th><th>Path</th><th>Status</th><th>Duration</th></tr>`)
		for i := len(requests) - 1; i >= 0; i-- {
			entry := requests[i]
			body.WriteString(fmt.Sprintf(
				`<tr class="%s"><td>%s</td><td>%s</td><td>%d</td><td>%s</td></tr>`,
				statusClass(entry.Status),
				template.HTMLEscapeString(entry.Method),
				template.HTMLEscapeString(entry.Path),
				entry.Status,
				entry.Duration.Round(time.Microsecond),
			))
		}
		body.WriteString(`</table>`)
	}

	body.WriteString(`<h2>Queries</h2>`)
	if len(queries) == 0 {
		body.WriteString(`<p class="empty">No queries recorded.</p>`)
	} else {
		body.WriteString(`<table><tr><th>Connection</th><th>SQL</th><th>Bindings</th><th>Duration</th></tr>`)
		for i := len(queries) - 1; i >= 0; i-- {
			entry := queries[i]

			bindings := fmt.Sprintf("%d", entry.BindingCount)
			if len(entry.Bindings) > 0 {
				bindings = template.HTMLEscapeString(strings.Join(entry.Bindings, ", "))
			}

			row := fmt.Sprintf(
				`<tr class="%s"><td>%s</td><td><code>%s</code></td><td>%s</td><td>%s</td></tr>`,
				queryClass(entry),
				template.HTMLEscapeString(entry.Connection),
				template.HTMLEscapeString(entry.SQL),
				bindings,
				entry.Duration.Round(time.Microsecond),
			)
			body.WriteString(row)

			if entry.Err != "" {
				body.WriteString(fmt.Sprintf(
					`<tr class="failed"><td colspan="4">%s</td></tr>`,
					template.HTMLEscapeString(entry.Err),
				))
			}
		}
		body.WriteString(`</table>`)
	}

	body.WriteString(panelFoot)
	return body.String()
}

// statusClass colours a row by response status.
func statusClass(status int) string {
	switch {
	case status >= 500:
		return "failed"
	case status >= 400:
		return "warned"
	default:
		return "ok"
	}
}

// queryClass marks failed and slow queries.
func queryClass(entry QueryEntry) string {
	if entry.Err != "" {
		return "failed"
	}
	if entry.Duration > 100*time.Millisecond {
		return "warned"
	}
	return "ok"
}

const panelHead = `<!doctype html><html><head><title>Genesys dev panel</title><style>
body{font:14px/1.5 ui-monospace,SFMono-Regular,Menlo,monospace;margin:2rem;color:#111;background:#fafafa}
h1{font-size:1.1rem}h2{font-size:1rem;margin-top:2rem}
table{border-collapse:collapse;width:100%;background:#fff}
th,td{border:1px solid #e5e5e5;padding:.35rem .6rem;text-align:left;vertical-align:top}
th{background:#f3f3f3}
tr.failed td{background:#fff0f0}tr.warned td{background:#fffaf0}
code{white-space:pre-wrap;word-break:break-word}
.empty{color:#777}
form{margin-top:1.5rem}
</style></head><body><h1>Genesys dev panel</h1>`

const panelFoot = `<form method="POST" action="clear"><button type="submit">Clear</button></form>
<p class="empty">Development only. Queries are attributed to a request by time window.</p>
</body></html>`
