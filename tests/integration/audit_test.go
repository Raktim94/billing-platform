//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/google/uuid"

	identityapp "rechvix/internal/modules/identity/app"
)

// TestAuditLogWrittenForLoginAndPasswordChange verifies brief §30's
// mandatory audit coverage actually lands rows in audit_log for two of
// the explicitly-listed sensitive actions, scoped correctly to the
// organisation that performed them.
func TestAuditLogWrittenForLoginAndPasswordChange(t *testing.T) {
	ctx := context.Background()
	identitySvc, _ := newTestIdentityService(t)

	email := "audit-" + uuid.NewString()[:8] + "@example.com"
	password := "correct horse battery staple 42"
	boot := bootstrapTestTenant(t, ctx, identitySvc, email, password)

	loginResult, err := identitySvc.Login(ctx, identityapp.LoginParams{Email: email, Password: password})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	principal, err := identitySvc.ValidateSession(ctx, loginResult.SessionToken)
	if err != nil {
		t.Fatalf("ValidateSession: %v", err)
	}

	if err := identitySvc.ChangePassword(ctx, principal, password, "brand new integration pw 1", "brand new integration pw 1"); err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}

	requireAuditAction(t, ctx, boot.OrganisationID, "user.created")
	requireAuditAction(t, ctx, boot.OrganisationID, "user.login")
	requireAuditAction(t, ctx, boot.OrganisationID, "user.password_changed")
}

func requireAuditAction(t *testing.T, ctx context.Context, orgID uuid.UUID, action string) {
	t.Helper()
	var count int
	err := sharedPool.RunScoped(ctx, orgID, func(ctx context.Context) error {
		row := sharedPool.Q(ctx).QueryRow(ctx,
			`SELECT count(*) FROM audit_log WHERE organisation_id = $1 AND action = $2`, orgID, action)
		return row.Scan(&count)
	})
	if err != nil {
		t.Fatalf("querying audit_log for action %q: %v", action, err)
	}
	if count == 0 {
		t.Fatalf("expected at least one audit_log row for action %q under organisation %s, found none", action, orgID)
	}
}
