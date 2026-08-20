package http

import (
	"strings"
)

// Domain groups routes under a host pattern, Laravel's subdomain
// routing. Pattern segments in braces capture into domain parameters:
//
//	router.Domain("{account}.example.com", func(r *http.Router) {
//	    r.GET("/dashboard", func(ctx *http.Context) error {
//	        account := http.DomainParam(ctx, "account")
//	        ...
//	    })
//	})
//
// Requests whose host does not match fall through to later routes with
// the same path (e.g. the apex domain's routes).
func (r *Router) Domain(pattern string, fn func(*Router)) {
	r.Group("", fn, domainMiddleware(pattern))
}

const domainParamPrefix = "_domain_param_"

// DomainParam returns a captured domain parameter ("" when absent).
func DomainParam(ctx *Context, name string) string {
	if value, ok := ctx.Get(domainParamPrefix + name).(string); ok {
		return value
	}
	return ""
}

// domainMiddleware matches the request host against the pattern; a
// mismatch skips to the next matching route instead of failing.
func domainMiddleware(pattern string) MiddlewareFunc {
	want := strings.Split(strings.ToLower(pattern), ".")
	return func(ctx *Context, next func() error) error {
		host := strings.ToLower(ctx.FiberCtx().Hostname())
		if i := strings.IndexByte(host, ':'); i >= 0 {
			host = host[:i]
		}

		params, ok := matchDomain(want, strings.Split(host, "."))
		if !ok {
			return ctx.FiberCtx().Next() // let later routes claim the path
		}
		for name, value := range params {
			ctx.Set(domainParamPrefix+name, value)
		}
		return next()
	}
}

// matchDomain compares host segments to the pattern, capturing {name}
// wildcards. Segment counts must match exactly.
func matchDomain(pattern, host []string) (map[string]string, bool) {
	if len(pattern) != len(host) {
		return nil, false
	}
	params := make(map[string]string)
	for i, segment := range pattern {
		if strings.HasPrefix(segment, "{") && strings.HasSuffix(segment, "}") {
			name := segment[1 : len(segment)-1]
			if host[i] == "" {
				return nil, false
			}
			params[name] = host[i]
			continue
		}
		if segment != host[i] {
			return nil, false
		}
	}
	return params, true
}
