// Package app is logistics' application/use-case layer.
package app

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"billing-platform/internal/modules/logistics/domain"
	"billing-platform/internal/platform/audit"
	"billing-platform/internal/platform/database"
	"billing-platform/internal/platform/permissions"
)

type Service struct {
	pool         database.Runner
	vehicles     domain.VehicleRepository
	transporters domain.TransporterRepository
	preferences  domain.PreferenceRepository
	permissions  *permissions.Checker
	audit        audit.Recorder
	now          func() time.Time
}

func NewService(pool database.Runner, vehicles domain.VehicleRepository, transporters domain.TransporterRepository,
	preferences domain.PreferenceRepository, checker *permissions.Checker, recorder audit.Recorder) *Service {
	return &Service{pool: pool, vehicles: vehicles, transporters: transporters, preferences: preferences,
		permissions: checker, audit: recorder, now: time.Now}
}

type CreateVehicleParams struct {
	RegistrationNumber   string
	Nickname             string
	VehicleType          string
	DefaultTransportMode string
}

func (s *Service) CreateVehicle(ctx context.Context, principal permissions.Principal, p CreateVehicleParams) (*domain.Vehicle, error) {
	if err := s.permissions.Require(ctx, principal, "logistics.manage", permissions.Scope{}); err != nil {
		return nil, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("logistics: generating vehicle id: %w", err)
	}
	now := s.now()
	v := &domain.Vehicle{ID: id, OrganisationID: principal.OrganisationID, RegistrationNumber: p.RegistrationNumber,
		Nickname: p.Nickname, VehicleType: p.VehicleType, DefaultTransportMode: p.DefaultTransportMode,
		Active: true, CreatedAt: now, UpdatedAt: now}
	err = s.pool.RunScoped(ctx, principal.OrganisationID, func(ctx context.Context) error {
		if err := s.vehicles.Create(ctx, v); err != nil {
			return err
		}
		return s.audit.Record(ctx, audit.Entry{OrganisationID: principal.OrganisationID, ActorUserID: &principal.UserID,
			ActorType: audit.ActorUser, Action: "vehicle.create", EntityType: "vehicle", EntityID: &v.ID})
	})
	if err != nil {
		return nil, err
	}
	return v, nil
}

func (s *Service) ListVehicles(ctx context.Context, principal permissions.Principal, activeOnly bool) ([]*domain.Vehicle, error) {
	if err := s.permissions.Require(ctx, principal, "logistics.view", permissions.Scope{}); err != nil {
		return nil, err
	}
	var out []*domain.Vehicle
	err := s.pool.RunScoped(ctx, principal.OrganisationID, func(ctx context.Context) error {
		var err error
		out, err = s.vehicles.ListByOrganisation(ctx, principal.OrganisationID, activeOnly)
		return err
	})
	return out, err
}

func (s *Service) DeactivateVehicle(ctx context.Context, principal permissions.Principal, id uuid.UUID) error {
	if err := s.permissions.Require(ctx, principal, "logistics.manage", permissions.Scope{}); err != nil {
		return err
	}
	return s.pool.RunScoped(ctx, principal.OrganisationID, func(ctx context.Context) error {
		return s.vehicles.Deactivate(ctx, id)
	})
}

type CreateTransporterParams struct {
	Name                 string
	TransporterID        string
	GSTIN                string
	Phone                string
	Address              string
	DefaultTransportMode string
}

func (s *Service) CreateTransporter(ctx context.Context, principal permissions.Principal, p CreateTransporterParams) (*domain.Transporter, error) {
	if err := s.permissions.Require(ctx, principal, "logistics.manage", permissions.Scope{}); err != nil {
		return nil, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("logistics: generating transporter id: %w", err)
	}
	now := s.now()
	t := &domain.Transporter{ID: id, OrganisationID: principal.OrganisationID, Name: p.Name,
		TransporterID: p.TransporterID, GSTIN: p.GSTIN, Phone: p.Phone, Address: p.Address,
		DefaultTransportMode: p.DefaultTransportMode, Active: true, CreatedAt: now, UpdatedAt: now}
	err = s.pool.RunScoped(ctx, principal.OrganisationID, func(ctx context.Context) error {
		if err := s.transporters.Create(ctx, t); err != nil {
			return err
		}
		return s.audit.Record(ctx, audit.Entry{OrganisationID: principal.OrganisationID, ActorUserID: &principal.UserID,
			ActorType: audit.ActorUser, Action: "transporter.create", EntityType: "transporter", EntityID: &t.ID})
	})
	if err != nil {
		return nil, err
	}
	return t, nil
}

func (s *Service) ListTransporters(ctx context.Context, principal permissions.Principal, activeOnly bool) ([]*domain.Transporter, error) {
	if err := s.permissions.Require(ctx, principal, "logistics.view", permissions.Scope{}); err != nil {
		return nil, err
	}
	var out []*domain.Transporter
	err := s.pool.RunScoped(ctx, principal.OrganisationID, func(ctx context.Context) error {
		var err error
		out, err = s.transporters.ListByOrganisation(ctx, principal.OrganisationID, activeOnly)
		return err
	})
	return out, err
}

func (s *Service) DeactivateTransporter(ctx context.Context, principal permissions.Principal, id uuid.UUID) error {
	if err := s.permissions.Require(ctx, principal, "logistics.manage", permissions.Scope{}); err != nil {
		return err
	}
	return s.pool.RunScoped(ctx, principal.OrganisationID, func(ctx context.Context) error {
		return s.transporters.Deactivate(ctx, id)
	})
}

// RecordTransportUsageForOtherModule is a cross-module write (Stage 8c) —
// same no-self-scoping convention as inventory.RecordMovementForOtherModule
// (docs/architecture.md §2): ewaybill's app layer calls this from inside
// its own already-open RunScoped block when a free-portal preparation
// or manual entry records a real vehicle/transporter choice, so the next
// invoice for the same customer can default to it.
func (s *Service) RecordTransportUsageForOtherModule(ctx context.Context, orgID, customerPartyID uuid.UUID, vehicleID, transporterID *uuid.UUID) error {
	if vehicleID == nil && transporterID == nil {
		return nil
	}
	return s.preferences.RecordUsage(ctx, orgID, customerPartyID, vehicleID, transporterID)
}

// SuggestForCustomerForOtherModule resolves the "smart default" (docs/
// architecture.md §9b) — the customer's most-recently-used vehicle/
// transporter, or (nil, nil, nil) if there's no history yet (not an
// error: a first-time customer simply has no default to suggest).
func (s *Service) SuggestForCustomerForOtherModule(ctx context.Context, orgID, customerPartyID uuid.UUID) (vehicleID, transporterID *uuid.UUID, err error) {
	pref, err := s.preferences.MostRecent(ctx, orgID, customerPartyID)
	if err != nil {
		if err == domain.ErrNotFound {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	return pref.VehicleID, pref.TransporterID, nil
}
