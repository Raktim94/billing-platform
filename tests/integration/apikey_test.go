//go:build integration

package integration

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	identityapp "rechvix/internal/modules/identity/app"
	identitydomain "rechvix/internal/modules/identity/domain"
	identityhttp "rechvix/internal/modules/identity/httpapi"
	httpx "rechvix/internal/platform/http"
	"rechvix/internal/platform/permissions"
)

func TestAPIKey_CreateThenAuthenticateAsAlternativeToSession(t *testing.T) {
	ctx := context.Background()
	identitySvc, _ := newTestIdentityService(t)
	boot := bootstrapTestTenant(t, ctx, identitySvc, "apikey-"+uuid.NewString()[:8]+"@example.com", "correct horse battery staple 42")
	principal := permissions.Principal{UserID: boot.OwnerUserID, OrganisationID: boot.OrganisationID}

	created, err := identitySvc.CreateAPIKey(ctx, principal, identityapp.CreateAPIKeyParams{
		Name: "test-key", Scopes: []identitydomain.APIScope{identitydomain.ScopeReportsRead},
	})
	if err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}
	if created.RawKey == "" {
		t.Fatal("expected a non-empty raw key")
	}

	resolved, scopes, err := identitySvc.ValidateAPIKey(ctx, created.RawKey, "")
	if err != nil {
		t.Fatalf("ValidateAPIKey: %v", err)
	}
	if resolved.OrganisationID != principal.OrganisationID || resolved.UserID != principal.UserID {
		t.Fatalf("ValidateAPIKey resolved wrong principal: got %+v, want org=%s user=%s", resolved, principal.OrganisationID, principal.UserID)
	}
	if len(scopes) != 1 || scopes[0] != identitydomain.ScopeReportsRead {
		t.Fatalf("ValidateAPIKey scopes = %v, want [reports:read]", scopes)
	}
}

func TestAPIKey_EmptyScopeListRejected(t *testing.T) {
	ctx := context.Background()
	identitySvc, _ := newTestIdentityService(t)
	boot := bootstrapTestTenant(t, ctx, identitySvc, "apikey-empty-"+uuid.NewString()[:8]+"@example.com", "correct horse battery staple 42")
	principal := permissions.Principal{UserID: boot.OwnerUserID, OrganisationID: boot.OrganisationID}

	_, err := identitySvc.CreateAPIKey(ctx, principal, identityapp.CreateAPIKeyParams{Name: "no-scopes"})
	if err == nil {
		t.Fatal("expected CreateAPIKey to reject an empty scope list (brief §36: never a wildcard/all-permissions default)")
	}
}

func TestAPIKey_RevokedKeyIsRejected(t *testing.T) {
	ctx := context.Background()
	identitySvc, _ := newTestIdentityService(t)
	boot := bootstrapTestTenant(t, ctx, identitySvc, "apikey-revoke-"+uuid.NewString()[:8]+"@example.com", "correct horse battery staple 42")
	principal := permissions.Principal{UserID: boot.OwnerUserID, OrganisationID: boot.OrganisationID}

	created, err := identitySvc.CreateAPIKey(ctx, principal, identityapp.CreateAPIKeyParams{
		Name: "revoke-me", Scopes: []identitydomain.APIScope{identitydomain.ScopeReportsRead},
	})
	if err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}
	if err := identitySvc.RevokeAPIKey(ctx, principal, created.APIKey.ID); err != nil {
		t.Fatalf("RevokeAPIKey: %v", err)
	}
	if _, _, err := identitySvc.ValidateAPIKey(ctx, created.RawKey, ""); err == nil {
		t.Fatal("expected ValidateAPIKey to reject a revoked key")
	}
}

// TestAPIKey_RESTMiddleware_AuthenticatesWithoutSessionCookie proves
// RequireAuthOrAPIKey (internal/modules/identity/httpapi/middleware.go)
// actually works as an alternative to a session cookie over real HTTP —
// not just that the underlying ValidateAPIKey function works in
// isolation.
func TestAPIKey_RESTMiddleware_AuthenticatesWithoutSessionCookie(t *testing.T) {
	ctx := context.Background()
	identitySvc, _ := newTestIdentityService(t)
	boot := bootstrapTestTenant(t, ctx, identitySvc, "apikey-mw-"+uuid.NewString()[:8]+"@example.com", "correct horse battery staple 42")
	principal := permissions.Principal{UserID: boot.OwnerUserID, OrganisationID: boot.OrganisationID}

	created, err := identitySvc.CreateAPIKey(ctx, principal, identityapp.CreateAPIKeyParams{
		Name: "mw-key", Scopes: []identitydomain.APIScope{identitydomain.ScopeReportsRead},
	})
	if err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}

	mux := http.NewServeMux()
	mux.Handle("/protected", identityhttp.RequireAuthOrAPIKey(identitySvc, "bp_test_session")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := httpx.PrincipalFromContext(r.Context())
		if !ok {
			t.Error("expected a Principal in context")
		}
		if p.OrganisationID != principal.OrganisationID {
			t.Error("resolved principal has the wrong organisation")
		}
		w.WriteHeader(http.StatusOK)
	})))

	srv := httptest.NewServer(mux)
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/protected", nil)
	req.Header.Set("Authorization", "Bearer "+created.RawKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 (API key alone, no session cookie, should authenticate)", resp.StatusCode)
	}

	// No Authorization header and no cookie must be rejected.
	req2, _ := http.NewRequest(http.MethodGet, srv.URL+"/protected", nil)
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatalf("request2: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status (no credentials) = %d, want 401", resp2.StatusCode)
	}
}
