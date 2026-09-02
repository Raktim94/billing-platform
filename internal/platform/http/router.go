package httpx

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

// RouterConfig controls the base middleware chain.
type RouterConfig struct {
	AllowedOrigins []string
	Logger         *slog.Logger
}

// NewRouter builds a chi.Mux with the platform's standard middleware
// chain already mounted: request ID, structured request logging, panic
// recovery, security headers, and CORS. Callers mount their own module
// routers (e.g. under /api/v1/) on top of this.
//
// Ordering matters: RequestID must run before RequestLogger (which reads
// the ID), and Recoverer must wrap everything downstream of it so a panic
// anywhere in a module handler is still caught.
func NewRouter(cfg RouterConfig) *chi.Mux {
	r := chi.NewRouter()
	// Deliberately NOT using chi/middleware.RealIP: it unconditionally
	// trusts X-Forwarded-For/X-Real-IP/True-Client-IP and rewrites
	// r.RemoteAddr from whichever is present — spoofable by any client
	// when nothing in front of this process actually sets/sanitizes those
	// headers (flagged by staticcheck SA1019, GHSA-3fxj-6jh8-hvhx). Since
	// docs/architecture.md §12 makes the reverse proxy optional and
	// deployment-specific, this process has no fixed way to know whether
	// those headers are trustworthy. Until a trusted-proxy-aware
	// resolver exists (only honor X-Forwarded-For from a configured list
	// of proxy CIDRs), r.RemoteAddr — the actual TCP peer address, which
	// cannot be spoofed at the network layer — is what audit logs and
	// rate limiting key on (see identity/httpapi's clientIP helper). The
	// documented cost: IP fields will show the reverse proxy's address,
	// not the original client's, on deployments that do put one in
	// front. Revisit once a specific hosting story needs real client IPs
	// behind a known proxy.
	r.Use(RequestID)
	r.Use(RequestLogger(cfg.Logger))
	r.Use(Recoverer)
	r.Use(SecurityHeaders)
	r.Use(CORS(cfg.AllowedOrigins))
	r.Use(chimiddleware.StripSlashes)

	r.Get("/health/live", LiveHandler())

	return r
}

// MountReady mounts the readiness endpoint once the checkers it depends
// on (e.g. the DB pool) exist — separated from NewRouter so the
// composition root controls exactly what's checked.
func MountReady(r chi.Router, checkers ...ReadinessChecker) {
	r.Get("/health/ready", func(w http.ResponseWriter, req *http.Request) {
		ReadyHandler(checkers...)(w, req)
	})
}
