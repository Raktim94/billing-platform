// Package app is the gstindia module's application layer: permission-
// checked, audited CRUD around tax_rate_master (docs/architecture.md §2).
// The calculation engine itself (internal/modules/gstindia.Engine) is
// wired directly into internal/modules/taxation as a TaxEngine — this
// package is only the admin-facing rate-configuration surface.
package app

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"billing-platform/internal/modules/gstindia/domain"
	"billing-platform/internal/platform/audit"
	"billing-platform/internal/platform/database"
	"billing-platform/internal/platform/permissions"
)

type Service struct {
	pool        database.Runner
	rates       domain.TaxRateRepository
	permissions *permissions.Checker
	audit       audit.Recorder
	now         func() time.Time
}

func NewService(pool database.Runner, rates domain.TaxRateRepository, checker *permissions.Checker, recorder audit.Recorder) *Service {
	return &Service{pool: pool, rates: rates, permissions: checker, audit: recorder, now: time.Now}
}

func (s *Service) view(ctx context.Context, principal permissions.Principal) error {
	return s.permissions.Require(ctx, principal, "gst.view", permissions.Scope{})
}

func (s *Service) manage(ctx context.Context, principal permissions.Principal) error {
	return s.permissions.Require(ctx, principal, "gst.manage", permissions.Scope{})
}

type CreateRateParams struct {
	HSNSACCode     string
	Classification domain.RateClassification
	GSTRate        decimal.Decimal
	CessRate       decimal.Decimal
	ValidFrom      time.Time
	ValidTo        *time.Time
}

// CreateRate adds a new tax_rate_master row. It does not validate against
// overlapping validity windows for the same HSN/SAC (see
// migrations/0015_gstindia.up.sql's comment) — the operator is trusted to
// manage rate history sanely, same trust level as brief §7's "valid_from/
// valid_to" requirement assumes; a stricter overlap-rejecting validation
// pass is a reasonable Stage 6+ hardening item, not core Stage 5 scope.
func (s *Service) CreateRate(ctx context.Context, principal permissions.Principal, p CreateRateParams) (*domain.TaxRateMaster, error) {
	if err := s.manage(ctx, principal); err != nil {
		return nil, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("gstindia: generating tax_rate_master id: %w", err)
	}
	now := s.now()
	m := &domain.TaxRateMaster{
		ID: id, OrganisationID: principal.OrganisationID, CountryCode: "IN",
		HSNSACCode: p.HSNSACCode, Classification: p.Classification,
		GSTRate: p.GSTRate, CessRate: p.CessRate,
		ValidFrom: p.ValidFrom, ValidTo: p.ValidTo, CreatedAt: now,
	}
	err = s.pool.RunScoped(ctx, principal.OrganisationID, func(ctx context.Context) error {
		if err := s.rates.Create(ctx, m); err != nil {
			return err
		}
		return s.audit.Record(ctx, audit.Entry{
			OrganisationID: principal.OrganisationID, ActorUserID: &principal.UserID, ActorType: audit.ActorUser,
			Action: "tax_rate_master.created", EntityType: "tax_rate_master", EntityID: &id,
			AfterState: map[string]any{"hsn_sac_code": p.HSNSACCode, "gst_rate": p.GSTRate.String(), "cess_rate": p.CessRate.String()},
			At:         now,
		})
	})
	if err != nil {
		return nil, err
	}
	return m, nil
}

func (s *Service) ListRatesByHSN(ctx context.Context, principal permissions.Principal, hsnSacCode string) ([]*domain.TaxRateMaster, error) {
	if err := s.view(ctx, principal); err != nil {
		return nil, err
	}
	var out []*domain.TaxRateMaster
	err := s.pool.RunScoped(ctx, principal.OrganisationID, func(ctx context.Context) error {
		var err error
		out, err = s.rates.ListByHSN(ctx, principal.OrganisationID, "IN", hsnSacCode)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
