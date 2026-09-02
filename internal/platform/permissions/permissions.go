// Package permissions implements RBAC permission checking
// (docs/architecture.md §10, brief §26). Every protected application-layer
// handler calls Checker.Require before doing anything — authorization is
// never inferred from what the UI happened to render (brief Rule 6).
package permissions

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"billing-platform/internal/platform/database"
)

// Principal is the authenticated caller of an application-layer method —
// resolved once by the HTTP layer from a validated session (or, later,
// API key) and passed down explicitly rather than pulled from context
// deep inside a module, so a use-case method's signature makes it obvious
// which calls require authentication.
type Principal struct {
	UserID         uuid.UUID
	OrganisationID uuid.UUID
}

// Scope describes how narrowly a requested action is targeted. A nil
// field means "this action isn't tied to that level" (e.g. viewing the
// organisation-wide dashboard has no branch/warehouse). Require matches a
// user's grants against this scope — see Checker.Require for the matching
// rule.
type Scope struct {
	LegalEntityID *uuid.UUID
	BranchID      *uuid.UUID
	WarehouseID   *uuid.UUID
}

// Grant is one row of "this user, through some role, holds this
// permission, optionally restricted to a specific legal entity/branch/
// warehouse." A nil field on a Grant means that grant is NOT restricted
// at that level (it applies regardless of which legal entity/branch/
// warehouse the action targets) — this is the opposite meaning of a nil
// field on Scope, which is why the matching rule in Require handles the
// two independently rather than doing a naive field-by-field equality.
type Grant struct {
	PermissionCode string
	LegalEntityID  *uuid.UUID
	BranchID       *uuid.UUID
	WarehouseID    *uuid.UUID
}

// Store loads a user's effective grants. Implemented against Postgres in
// pg.go; kept as an interface so unit tests can supply an in-memory fake
// without a database.
type Store interface {
	Grants(ctx context.Context, userID uuid.UUID) ([]Grant, error)
}

// ErrForbidden is wrapped into the error Require returns when the
// principal lacks the requested permission. Callers map it to HTTP 403
// (internal/platform/http.NewForbidden) at the transport boundary.
type ErrForbidden struct {
	PermissionCode string
}

func (e *ErrForbidden) Error() string {
	return fmt.Sprintf("permissions: missing %q", e.PermissionCode)
}

// Checker checks a user's permissions against their loaded grants. It
// holds a database.Runner and opens its OWN short RunScoped transaction
// around the grants lookup — deliberately, rather than trusting every
// caller to already be inside one. user_roles and roles (which Store.Grants
// joins across) are RLS-protected tables (migrations/0002_rbac_catalog.up.sql,
// 0003_users.up.sql): a Require call made outside any
// app.current_organisation_id scope would see zero grants and reject
// every request as forbidden, fail-closed but wrongly — Stage 2's own
// integration tests caught exactly this bug when Require was first
// wired up called before the caller's RunScoped block instead of inside
// it. Self-scoping here means every module gets this right automatically,
// instead of every future call site needing to remember the ordering.
type Checker struct {
	store  Store
	runner database.Runner
}

func NewChecker(store Store, runner database.Runner) *Checker {
	return &Checker{store: store, runner: runner}
}

// Require returns nil if principal holds permissionCode at (or covering)
// scope, otherwise a non-nil error wrapping ErrForbidden.
//
// A grant matches a requested scope level-by-level: for each of
// LegalEntityID/BranchID/WarehouseID, either the grant's value at that
// level is nil (unrestricted — applies everywhere) or it equals the
// scope's value at that level. A grant scoped to a specific branch never
// matches a request naming a different branch, but an organisation-wide
// grant (all three nil) matches any scope.
func (c *Checker) Require(ctx context.Context, principal Principal, permissionCode string, scope Scope) error {
	var grants []Grant
	err := c.runner.RunScoped(ctx, principal.OrganisationID, func(ctx context.Context) error {
		var err error
		grants, err = c.store.Grants(ctx, principal.UserID)
		return err
	})
	if err != nil {
		return fmt.Errorf("permissions: loading grants: %w", err)
	}
	for _, g := range grants {
		if g.PermissionCode != permissionCode {
			continue
		}
		if levelMatches(g.LegalEntityID, scope.LegalEntityID) &&
			levelMatches(g.BranchID, scope.BranchID) &&
			levelMatches(g.WarehouseID, scope.WarehouseID) {
			return nil
		}
	}
	return fmt.Errorf("permissions: user %s: %w", principal.UserID, &ErrForbidden{PermissionCode: permissionCode})
}

func levelMatches(grantValue, scopeValue *uuid.UUID) bool {
	if grantValue == nil {
		return true // unrestricted at this level
	}
	if scopeValue == nil {
		// Grant is restricted to a specific entity at this level, but the
		// requested action doesn't specify one — treat as non-matching
		// rather than guessing; callers that need org-wide semantics
		// should hold an org-wide (nil) grant.
		return false
	}
	return *grantValue == *scopeValue
}
