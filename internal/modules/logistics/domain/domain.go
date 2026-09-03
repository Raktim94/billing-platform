// Package domain is vehicle/transporter master data (docs/architecture.md
// §9b: "New master data") — plain org-scoped reference data supporting
// the e-Way Bill transport fields, deliberately its own small module
// rather than folded into ewaybill (a vehicle/transporter is reusable
// reference data, not an e-Way-Bill-specific concept).
package domain

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

type Vehicle struct {
	ID                   uuid.UUID
	OrganisationID       uuid.UUID
	RegistrationNumber   string
	Nickname             string
	VehicleType          string
	DefaultTransportMode string
	Active               bool
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

type Transporter struct {
	ID                   uuid.UUID
	OrganisationID       uuid.UUID
	Name                 string
	TransporterID        string
	GSTIN                string
	Phone                string
	Address              string
	DefaultTransportMode string
	Active               bool
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// Preference is one (customer, vehicle, transporter) combination's usage
// history — RecordUsage upserts it (increment used_count, bump
// last_used_at); PreferredFor resolves the "smart default" (docs/
// architecture.md §9b) as a plain most-recently-used lookup.
type Preference struct {
	ID              uuid.UUID
	OrganisationID  uuid.UUID
	CustomerPartyID uuid.UUID
	VehicleID       *uuid.UUID
	TransporterID   *uuid.UUID
	UsedCount       int
	LastUsedAt      time.Time
}

type VehicleRepository interface {
	Create(ctx context.Context, v *Vehicle) error
	GetByID(ctx context.Context, id uuid.UUID) (*Vehicle, error)
	ListByOrganisation(ctx context.Context, orgID uuid.UUID, activeOnly bool) ([]*Vehicle, error)
	Deactivate(ctx context.Context, id uuid.UUID) error
}

type TransporterRepository interface {
	Create(ctx context.Context, t *Transporter) error
	GetByID(ctx context.Context, id uuid.UUID) (*Transporter, error)
	ListByOrganisation(ctx context.Context, orgID uuid.UUID, activeOnly bool) ([]*Transporter, error)
	Deactivate(ctx context.Context, id uuid.UUID) error
}

type PreferenceRepository interface {
	// RecordUsage upserts (org, customer, vehicle, transporter) — either
	// or both of vehicleID/transporterID may be nil (a customer might
	// have a preferred transporter but no fixed vehicle, or vice versa).
	RecordUsage(ctx context.Context, orgID, customerPartyID uuid.UUID, vehicleID, transporterID *uuid.UUID) error
	// MostRecent returns the single most-recently-used preference row for
	// this customer, or ErrNotFound if the customer has no history yet.
	MostRecent(ctx context.Context, orgID, customerPartyID uuid.UUID) (*Preference, error)
}

var ErrNotFound = errors.New("logistics: record not found")
