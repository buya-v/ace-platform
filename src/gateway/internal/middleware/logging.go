package middleware

import (
	"encoding/json"
	"log"
	"net/http"
	"time"
)

// statusWriter wraps http.ResponseWriter to capture the status code.
type statusWriter struct {
	http.ResponseWriter
	status int
	size   int
}

func (sw *statusWriter) WriteHeader(status int) {
	sw.status = status
	sw.ResponseWriter.WriteHeader(status)
}

func (sw *statusWriter) Write(b []byte) (int, error) {
	n, err := sw.ResponseWriter.Write(b)
	sw.size += n
	return n, err
}

// logEntry is the structured JSON log format.
type logEntry struct {
	Timestamp  string `json:"timestamp"`
	Level      string `json:"level"`
	Method     string `json:"method"`
	Path       string `json:"path"`
	Status     int    `json:"status"`
	Duration   string `json:"duration_ms"`
	Size       int    `json:"response_bytes"`
	RemoteAddr string `json:"remote_addr"`
	RequestID  string `json:"request_id,omitempty"`
	UserID     string `json:"user_id,omitempty"`
}

// Logging creates structured JSON logging middleware.
func Logging() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}

			next.ServeHTTP(sw, r)

			duration := time.Since(start)
			entry := logEntry{
				Timestamp:  start.UTC().Format(time.RFC3339Nano),
				Level:      "info",
				Method:     r.Method,
				Path:       r.URL.Path,
				Status:     sw.status,
				Duration:   duration.String(),
				Size:       sw.size,
				RemoteAddr: r.RemoteAddr,
				RequestID:  RequestIDFromContext(r.Context()),
			}

			if claims := ClaimsFromContext(r.Context()); claims != nil {
				entry.UserID = claims.Sub
			}

			if sw.status >= 500 {
				entry.Level = "error"
			} else if sw.status >= 400 {
				entry.Level = "warn"
			}

			data, err := json.Marshal(entry)
			if err != nil {
				log.Printf("failed to marshal log entry: %v", err)
				return
			}
			log.Println(string(data))
		})
	}
}
