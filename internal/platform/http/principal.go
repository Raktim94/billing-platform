package httpx

import (
	"context"

	"billing-platform/internal/platform/permissions"
)

var principalContextKey = &contextKey{"httpx.principal"}

// WithPrincipal attaches the authenticated caller to ctx. Called once by
// the session-auth middleware (internal/modules/identity/httpapi); read
// by every module's HTTP handlers via PrincipalFromContext, so "who is
// making this request" has one shared representation across modules
// instead of each module inventing its own.
func WithPrincipal(ctx context.Context, p permissions.Principal) context.Context {
	return context.WithValue(ctx, principalContextKey, p)
}

// PrincipalFromContext returns the authenticated caller, if the request
// passed through the session-auth middleware.
func PrincipalFromContext(ctx context.Context) (permissions.Principal, bool) {
	p, ok := ctx.Value(principalContextKey).(permissions.Principal)
	return p, ok
}
