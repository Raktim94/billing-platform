// Package httpx is internal/platform/http's Go package (named httpx, not
// http, so files that need both this package and the standard library's
// net/http never hit an import-identifier collision).
package httpx

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"billing-platform/internal/platform/logging"
)

// AppError is the error type application/use-case handlers return when
// they want to control the HTTP status code and client-facing message.
// Any other error reaching WriteError is treated as an unexpected
// internal error: logged in full, but never exposed to the client beyond
// a generic message and a request ID (brief §57 — "Never expose stack
// traces to ordinary users").
type AppError struct {
	Status  int
	Code    string
	Message string
	Details map[string]any
	// Cause is the underlying error, logged server-side but never
	// serialized to the client.
	Cause error
}

func (e *AppError) Error() string {
	if e.Cause != nil {
		return e.Code + ": " + e.Message + ": " + e.Cause.Error()
	}
	return e.Code + ": " + e.Message
}

func (e *AppError) Unwrap() error { return e.Cause }

// Common, reusable application errors. Handlers construct more specific
// ones (e.g. with Details) inline as needed.
func NewNotFound(code, message string) *AppError {
	return &AppError{Status: http.StatusNotFound, Code: code, Message: message}
}

func NewBadRequest(code, message string) *AppError {
	return &AppError{Status: http.StatusBadRequest, Code: code, Message: message}
}

func NewUnauthorized(code, message string) *AppError {
	return &AppError{Status: http.StatusUnauthorized, Code: code, Message: message}
}

func NewForbidden(code, message string) *AppError {
	return &AppError{Status: http.StatusForbidden, Code: code, Message: message}
}

func NewConflict(code, message string) *AppError {
	return &AppError{Status: http.StatusConflict, Code: code, Message: message}
}

// errorEnvelope is the consistent JSON error shape required by brief §35.
type errorEnvelope struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	Details   map[string]any `json:"details,omitempty"`
	RequestID string         `json:"request_id"`
}

// WriteError writes a JSON error response. If err is (or wraps) an
// *AppError, its Status/Code/Message/Details are used verbatim. Any other
// error is logged at ERROR level with full detail and reduced, for the
// client, to a generic 500 with no information beyond a request ID —
// exactly the "actionable but non-destructive, no stack traces" shape
// brief §57 requires.
func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	requestID := RequestIDFromContext(r.Context())
	logger := logging.FromContext(r.Context())

	var appErr *AppError
	if errors.As(err, &appErr) {
		if appErr.Cause != nil {
			logger.ErrorContext(r.Context(), "request failed",
				slog.String("code", appErr.Code),
				slog.String("request_id", requestID),
				slog.Any("cause", appErr.Cause))
		}
		writeJSON(w, appErr.Status, errorEnvelope{Error: errorBody{
			Code:      appErr.Code,
			Message:   appErr.Message,
			Details:   appErr.Details,
			RequestID: requestID,
		}})
		return
	}

	logger.ErrorContext(r.Context(), "unhandled internal error",
		slog.String("request_id", requestID),
		slog.Any("error", err))
	writeJSON(w, http.StatusInternalServerError, errorEnvelope{Error: errorBody{
		Code:      "INTERNAL_ERROR",
		Message:   "An unexpected error occurred. Contact support with the request ID if this persists.",
		RequestID: requestID,
	}})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// WriteJSON writes a successful JSON response body.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	writeJSON(w, status, v)
}
