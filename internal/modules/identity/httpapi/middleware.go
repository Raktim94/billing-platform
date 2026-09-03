// Package httpapi is the identity module's HTTP transport layer — it
// translates chi requests into calls on app.Service and back, and holds
// nothing else (docs/architecture.md §2: HTTP layer, DTOs only).
package httpapi

import (
	"errors"
	"net/http"

	"billing-platform/internal/modules/identity/app"
	"billing-platform/internal/modules/identity/domain"
	httpx "billing-platform/internal/platform/http"
	"billing-platform/internal/platform/permissions"
)

// RequireAuth resolves the session cookie to a Principal and attaches it
// to the request context, or writes 401 if the cookie is missing or the
// session is invalid/expired/revoked. Mount this only on routes that
// require authentication — login, bootstrap, and the password-reset
// request/completion endpoints must stay reachable without a session.
func RequireAuth(svc *app.Service, cookieName string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(cookieName)
			if err != nil || cookie.Value == "" {
				httpx.WriteError(w, r, httpx.NewUnauthorized("UNAUTHENTICATED", "Sign in required."))
				return
			}
			principal, err := svc.ValidateSession(r.Context(), cookie.Value)
			if err != nil {
				if errors.Is(err, domain.ErrSessionInvalid) {
					httpx.WriteError(w, r, httpx.NewUnauthorized("SESSION_INVALID", "Your session has expired. Please sign in again."))
					return
				}
				httpx.WriteError(w, r, err)
				return
			}
			ctx := httpx.WithPrincipal(r.Context(), principal)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireAuthOrAPIKey is RequireAuth's superset (docs/architecture.md
// §11, brief §36): a request authenticates via EITHER a valid session
// cookie OR an `Authorization: Bearer <key>` API key. Whichever succeeds
// produces the exact same permissions.Principal shape and is attached to
// ctx the same way — every downstream handler and permissions.Checker
// call is identical either way. An API-key-authenticated request
// additionally carries a scope restriction (permissions.
// WithAPIKeyScopeRestriction) that Require intersects with the owning
// user's real RBAC grants; a session-authenticated request carries none.
//
// The session cookie is tried first (the common case — a browser sending
// both a stale/absent cookie and no Authorization header should not pay
// for a wasted API-key DB lookup), then the bearer header.
func RequireAuthOrAPIKey(svc *app.Service, cookieName string) func(http.Handler) http.Handler {
	sessionOnly := RequireAuth(svc, cookieName)
	return func(next http.Handler) http.Handler {
		bySession := sessionOnly(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if cookie, err := r.Cookie(cookieName); err == nil && cookie.Value != "" {
				bySession.ServeHTTP(w, r)
				return
			}
			rawKey, ok := bearerToken(r)
			if !ok {
				httpx.WriteError(w, r, httpx.NewUnauthorized("UNAUTHENTICATED", "Sign in required."))
				return
			}
			principal, scopes, err := svc.ValidateAPIKey(r.Context(), rawKey, clientIP(r))
			if err != nil {
				httpx.WriteError(w, r, httpx.NewUnauthorized("API_KEY_INVALID", "Invalid or expired API key."))
				return
			}
			scopeStrings := make([]string, len(scopes))
			for i, s := range scopes {
				scopeStrings[i] = string(s)
			}
			ctx := httpx.WithPrincipal(r.Context(), principal)
			ctx = permissions.WithAPIKeyScopeRestriction(ctx, permissions.PermissionsForScopes(scopeStrings))
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func bearerToken(r *http.Request) (string, bool) {
	const prefix = "Bearer "
	h := r.Header.Get("Authorization")
	if len(h) <= len(prefix) || h[:len(prefix)] != prefix {
		return "", false
	}
	return h[len(prefix):], true
}
