// Package logger provides a structured JSON logger built on log/slog and
// helpers for propagating a request-scoped logger through context.Context.
//
// Every log line is a single JSON object written to stdout, making it
// compatible with CloudWatch Logs Insights, Datadog, jq, and similar tools.
//
// # Schema
//
// All events share the base fields emitted by slog:
//
//	{"time":"…","level":"INFO","msg":"…"}
//
// HTTP request events (logged by middleware/logger.go) add:
//
//	"request_id", "method", "path", "status", "duration_ms",
//	"remote_ip", "user_agent", "bytes"
//
// Business events logged from handlers add:
//
//	"request_id", "adspot_id", "placement"
//
// Error events add:
//
//	"request_id", "error"
//
// # Example queries
//
//	# All 5xx errors
//	cat app.log | jq 'select(.status >= 500)'
//
//	# Slow requests (> 500 ms)
//	cat app.log | jq 'select(.duration_ms > 500)'
//
//	# Full trace of a single request
//	cat app.log | jq 'select(.request_id == "abc-123")'
//
//	# Error rate by path
//	cat app.log | jq 'select(.msg == "request") | {path, status}'
package logger

import (
	"context"
	"log/slog"
	"os"
	"strings"
)

// contextKey is the unexported key used to store the logger in a context.
type contextKey struct{}

// New creates a JSON logger that writes to stdout at the given level.
// level is case-insensitive; accepted values: "debug", "info", "warn", "error".
// Any unknown value defaults to INFO.
func New(level string) *slog.Logger {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn", "warning":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}

	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: lvl,
		// Include source file/line only at debug level to keep production logs lean.
		AddSource: lvl == slog.LevelDebug,
	})
	return slog.New(handler)
}

// WithContext returns a copy of ctx that carries l.
// Use FromContext to retrieve it later.
func WithContext(ctx context.Context, l *slog.Logger) context.Context {
	return context.WithValue(ctx, contextKey{}, l)
}

// FromContext returns the logger stored in ctx by WithContext.
// If no logger is present, it falls back to slog.Default() so callers never
// need to guard against a nil logger.
func FromContext(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(contextKey{}).(*slog.Logger); ok && l != nil {
		return l
	}
	return slog.Default()
}
