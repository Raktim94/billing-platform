package httpx

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	"billing-platform/internal/platform/logging"
)

type contextKey struct{ name string }

var requestIDContextKey = &contextKey{"httpx.request_id"}

// RequestID assigns a UUIDv4 request ID (a request ID has no need to be
// time-ordered/sortable the way a stored entity's primary key does, so
// plain random is fine here) to every request, storing it in the context
// and echoing it back as the X-Request-Id response header. Every log line
// and error envelope for this request carries the same ID (brief §57).
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := uuid.New().String()
		w.Header().Set("X-Request-Id", id)
		ctx := context.WithValue(r.Context(), requestIDContextKey, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequestIDFromContext returns the current request's ID, or "" if none is
// set (e.g. called outside a request, such as from a background job).
func RequestIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDContextKey).(string); ok {
		return id
	}
	return ""
}

// RequestLogger attaches a request-scoped slog.Logger (carrying
// request_id) to the request context, and emits one structured log line
// per request with method, path, status, and latency.
func RequestLogger(base *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestID := RequestIDFromContext(r.Context())
			logger := base.With(slog.String("request_id", requestID))
			ctx := logging.WithContext(r.Context(), logger)

			start := time.Now()
			sw := &statusCapturingWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(sw, r.WithContext(ctx))

			logger.InfoContext(ctx, "http_request",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", sw.status),
				slog.Duration("duration", time.Since(start)),
			)
		})
	}
}

type statusCapturingWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusCapturingWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

// Recoverer converts a panic in any downstream handler into a logged
// error and a generic 500 response — never a crashed process, and never a
// stack trace leaked to the client (brief §57).
func Recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				logger := logging.FromContext(r.Context())
				logger.ErrorContext(r.Context(), "panic recovered",
					slog.String("request_id", RequestIDFromContext(r.Context())),
					slog.Any("panic", rec))
				WriteError(w, r, &AppError{
					Status:  http.StatusInternalServerError,
					Code:    "INTERNAL_ERROR",
					Message: "An unexpected error occurred. Contact support with the request ID if this persists.",
				})
			}
		}()
		next.ServeHTTP(w, r)
	})
}
