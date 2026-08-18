package main

import (
	"log/slog"
	"net/http"
	"time"
)

// statusRecorder wraps http.ResponseWriter to capture the status code
// written by the handler, since http.ResponseWriter doesn't expose it
// after the fact.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// withLogging logs one line per request: method, path, resulting status,
// and duration. It deliberately never logs headers, query strings, or
// credentials.
func withLogging(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		logger.Info("proxied request",
			"method", r.Method,
			"path", r.URL.Path,
			"upstream_status", rec.status,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}
