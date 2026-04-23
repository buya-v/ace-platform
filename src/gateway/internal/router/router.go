package router

import (
	"net/http"
	"strings"
)

// Route represents a single API route.
type Route struct {
	Method      string
	Pattern     string   // e.g. "/api/v1/orders/{order_id}"
	Backend     string   // e.g. "matching-engine"
	RPCMethod   string   // e.g. "OrderService/SubmitOrder"
	AuthLevel   string   // "none", "any", "trader", "clearing_admin", "exchange_admin", "compliance"
	RateGroup   string   // rate limit group
	Handler     http.HandlerFunc
}

// Router is a simple HTTP router with path parameter extraction.
type Router struct {
	routes     []Route
	notFound   http.HandlerFunc
}

// New creates a new Router.
func New() *Router {
	return &Router{
		notFound: func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"error":{"code":"NOT_FOUND","message":"Endpoint not found"}}`))
		},
	}
}

// Handle registers a route.
func (rt *Router) Handle(method, pattern string, handler http.HandlerFunc) {
	rt.routes = append(rt.routes, Route{
		Method:  method,
		Pattern: pattern,
		Handler: handler,
	})
}

// AddRoute registers a fully specified route.
func (rt *Router) AddRoute(route Route) {
	rt.routes = append(rt.routes, route)
}

// GetRoutes returns all registered routes (for testing/inspection).
func (rt *Router) GetRoutes() []Route {
	return rt.routes
}

// RouteExists reports whether any registered route has a pattern matching path,
// regardless of HTTP method. Used by the pre-auth 404 guard to reject requests
// to unknown paths before auth middleware runs.
func (rt *Router) RouteExists(path string) bool {
	for _, route := range rt.routes {
		if _, ok := matchPath(route.Pattern, path); ok {
			return true
		}
	}
	return false
}

// ServeHTTP implements http.Handler.
func (rt *Router) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	// Try to match a route
	for _, route := range rt.routes {
		if route.Method != r.Method {
			continue
		}
		params, ok := matchPath(route.Pattern, path)
		if !ok {
			continue
		}

		// Store path params in query (accessible via r.URL.Query())
		q := r.URL.Query()
		for k, v := range params {
			q.Set(k, v)
		}
		r.URL.RawQuery = q.Encode()

		route.Handler.ServeHTTP(w, r)
		return
	}

	// Check if path matches but method doesn't
	for _, route := range rt.routes {
		if _, ok := matchPath(route.Pattern, path); ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusMethodNotAllowed)
			w.Write([]byte(`{"error":{"code":"METHOD_NOT_ALLOWED","message":"Method not allowed"}}`))
			return
		}
	}

	rt.notFound(w, r)
}

// matchPath matches a URL path against a pattern with {param} placeholders.
// Returns extracted parameters and whether the path matched.
func matchPath(pattern, path string) (map[string]string, bool) {
	patternParts := splitPath(pattern)
	pathParts := splitPath(path)

	if len(patternParts) != len(pathParts) {
		return nil, false
	}

	params := make(map[string]string)
	for i, pp := range patternParts {
		if strings.HasPrefix(pp, "{") && strings.HasSuffix(pp, "}") {
			paramName := pp[1 : len(pp)-1]
			params[paramName] = pathParts[i]
		} else if pp != pathParts[i] {
			return nil, false
		}
	}
	return params, true
}

func splitPath(path string) []string {
	path = strings.Trim(path, "/")
	if path == "" {
		return nil
	}
	return strings.Split(path, "/")
}
