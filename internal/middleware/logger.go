package middleware

import (
	"log/slog"
	"net/http"
	"time"

	chi "github.com/go-chi/chi/v5/middleware"

	applogger "github.com/adspot-backend/adspot-backend/internal/logger"
)

// responseWriter wraps http.ResponseWriter to capture the status code and the
// number of bytes written so the request logger can include them in the log line.
type responseWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (rw *responseWriter) WriteHeader(status int) {
	rw.status = status
	rw.ResponseWriter.WriteHeader(status)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.bytes += n
	return n, err
}

// Logger is an HTTP middleware that:
//  1. Creates a request-scoped *slog.Logger enriched with request_id and
//     remote_ip, then stores it in the context so handlers can retrieve it
//     via logger.FromContext(ctx).
//  2. After the handler returns, emits a single JSON log line with the full
//     HTTP request summary (method, path, status, duration_ms, bytes, …).
//
// It is designed to replace chiMiddleware.Logger.
func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Enrich the logger with fields that are constant for this request.
		reqLogger := slog.Default().With(
			slog.String("request_id", chi.GetReqID(r.Context())),
			slog.String("remote_ip", r.RemoteAddr),
		)

		// Propagate the enriched logger through the context.
		ctx := applogger.WithContext(r.Context(), reqLogger)
		r = r.WithContext(ctx)

		// Wrap the ResponseWriter so we can observe what the handler writes.
		wrapped := &responseWriter{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(wrapped, r)

		// Emit one structured log line per request.
		reqLogger.LogAttrs(r.Context(), slog.LevelInfo, "request",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", wrapped.status),
			slog.Int64("duration_ms", time.Since(start).Milliseconds()),
			slog.String("user_agent", r.UserAgent()),
			slog.Int("bytes", wrapped.bytes),
		)
	})
}
