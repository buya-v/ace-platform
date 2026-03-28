package middleware

import (
	"net/http"

	"github.com/ace-platform/gateway/internal/types"
)

// BodyLimit limits the size of request bodies.
func BodyLimit(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body != nil && r.ContentLength > maxBytes {
				reqID := RequestIDFromContext(r.Context())
				types.WriteError(w, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE",
					"Request body too large", reqID)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			next.ServeHTTP(w, r)
		})
	}
}
