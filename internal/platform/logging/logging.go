// Package logging provides structured, context-propagated JSON logging via
// the standard library's log/slog. There is no other logging mechanism in
// this codebase — every package logs through here so log shape (JSON,
// request_id correlation) is uniform (brief §57, §58).
package logging

import (
	"context"
	"io"
	"log/slog"
	"os"
)

type contextKey struct{ name string }

var loggerContextKey = &contextKey{"logging.logger"}

// New builds a JSON slog.Logger writing to w at the given level ("debug",
// "info", "warn", "error"; unrecognized values fall back to "info").
func New(w io.Writer, level string) *slog.Logger {
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level: parseLevel(level),
	}))
}

// NewDefault builds a JSON logger to stdout at the given level. Use this
// in apps/server and apps/worker's composition root.
func NewDefault(level string) *slog.Logger {
	return New(os.Stdout, level)
}

func parseLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// WithContext returns a new context carrying logger. Middleware calls this
// once per request after attaching request_id/organisation_id/user_id
// fields, so every downstream call to FromContext gets a logger that
// already carries those correlation fields without threading them through
// every function signature.
func WithContext(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerContextKey, logger)
}

// FromContext returns the logger attached to ctx, or slog.Default() if
// none was attached. It never returns nil, so callers never need a nil
// check before logging.
func FromContext(ctx context.Context) *slog.Logger {
	if logger, ok := ctx.Value(loggerContextKey).(*slog.Logger); ok && logger != nil {
		return logger
	}
	return slog.Default()
}
