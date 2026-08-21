package http

// Health registers a liveness endpoint (default "/up") that answers
// 200 {"status":"ok"} - Laravel 11's health route. Point load balancer
// and uptime checks at it:
//
//	router.Health()          // GET /up
//	router.Health("/healthz")
func (r *Router) Health(path ...string) *Route {
	target := "/up"
	if len(path) > 0 && path[0] != "" {
		target = path[0]
	}
	return r.GET(target, func(ctx *Context) error {
		return ctx.JSONResponse(map[string]any{"status": "ok"})
	})
}
