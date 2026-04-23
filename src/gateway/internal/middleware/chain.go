package middleware

import "net/http"

// Chain applies a list of middleware to an http.Handler.
// Middleware are applied in order, so the first middleware in the list
// is the outermost wrapper (executes first on request, last on response).
func Chain(h http.Handler, middlewares ...func(http.Handler) http.Handler) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		h = middlewares[i](h)
	}
	return h
}
