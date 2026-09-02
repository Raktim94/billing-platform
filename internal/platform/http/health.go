package httpx

import (
	"context"
	"net/http"
)

// ReadinessChecker reports whether a dependency (the database pool, etc.)
// is currently usable. database.Pool implements this.
type ReadinessChecker interface {
	Ready(ctx context.Context) error
}

// LiveHandler always returns 200 if the process is up and serving HTTP at
// all (brief §58 — liveness must not depend on downstream health, or a
// transient DB blip would cause an orchestrator to kill a perfectly good
// process).
func LiveHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		WriteJSON(w, http.StatusOK, map[string]string{"status": "live"})
	}
}

// ReadyHandler returns 200 only if every checker reports healthy —
// currently just the database, per brief §58 ("Readiness checks database
// connectivity").
func ReadyHandler(checkers ...ReadinessChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		for _, c := range checkers {
			if err := c.Ready(r.Context()); err != nil {
				WriteJSON(w, http.StatusServiceUnavailable, map[string]string{
					"status": "not_ready",
					"error":  err.Error(),
				})
				return
			}
		}
		WriteJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	}
}
