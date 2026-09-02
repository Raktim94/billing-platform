// Package domain holds the contacts module's entity types and repository
// interfaces (docs/architecture.md §2, brief §16).
package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type PartyType string

const (
	PartyCustomer PartyType = "CUSTOMER"
	PartySupplier PartyType = "SUPPLIER"
	PartyBoth     PartyType = "BOTH"
)

type Status string

const (
	StatusActive   Status = "ACTIVE"
	StatusInactive Status = "INACTIVE"
)

type AddressType string

const (
	AddressBilling          AddressType = "BILLING"
	AddressShipping         AddressType = "SHIPPING"
	AddressWarehouse        AddressType = "WAREHOUSE"
	AddressRegisteredOffice AddressType = "REGISTERED_OFFICE"
)

// ValidPartyType reports whether t is one of the three party types the
// database CHECK constraint (migrations/0009_contacts.up.sql) allows.
// Checked at the application layer too (app.Service.CreateParty) so an
// invalid value comes back as a clean 400 rather than a raw driver error
// bubbling up from the database.
func ValidPartyType(t PartyType) bool {
	switch t {
	case PartyCustomer, PartySupplier, PartyBoth:
		return true
	default:
		return false
	}
}

// ValidAddressType reports whether t is one of the four address types the
// database CHECK constraint allows. Same rationale as ValidPartyType.
func ValidAddressType(t AddressType) bool {
	switch t {
	case AddressBilling, AddressShipping, AddressWarehouse, AddressRegisteredOffice:
		return true
	default:
		return false
	}
}

type Party struct {
	ID                uuid.UUID
	OrganisationID    uuid.UUID
	PartyType         PartyType
	LegalName         string
	TradeName         string
	Phone             string
	Email             string
	CurrencyCode      string
	CreditLimitAmount *decimal.Decimal
	PaymentTermsDays  *int
	Notes             string
	Status            Status
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type Address struct {
	ID             uuid.UUID
	OrganisationID uuid.UUID
	PartyID        uuid.UUID
	AddressType    AddressType
	Line1          string
	Line2          string
	City           string
	State          string
	PostalCode     string
	CountryCode    string
	IsDefault      bool
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// TaxRegistration is deliberately country-agnostic (docs/architecture.md
// §8) — RegistrationNumber holds a GSTIN for India today, and a VAT/EIN/
// other country's tax ID later, without a schema change.
type TaxRegistration struct {
	ID                 uuid.UUID
	OrganisationID     uuid.UUID
	PartyID            uuid.UUID
	CountryCode        string
	RegistrationNumber string
	StateCode          string
	IsPrimary          bool
	CreatedAt          time.Time
}

type PartyRepository interface {
	Create(ctx context.Context, p *Party) error
	GetByID(ctx context.Context, id uuid.UUID) (*Party, error)
	ListByOrganisation(ctx context.Context, orgID uuid.UUID) ([]*Party, error)
	// SearchByName is the fast fuzzy lookup used by billing-counter
	// customer search and supplier lookup (brief §24/§25).
	SearchByName(ctx context.Context, orgID uuid.UUID, query string, limit int) ([]*Party, error)
}

type AddressRepository interface {
	Create(ctx context.Context, a *Address) error
	ListByParty(ctx context.Context, partyID uuid.UUID) ([]*Address, error)
}

type TaxRegistrationRepository interface {
	Create(ctx context.Context, t *TaxRegistration) error
	ListByParty(ctx context.Context, partyID uuid.UUID) ([]*TaxRegistration, error)
	// GetByRegistrationNumber is the exact-match GSTIN lookup (brief §24).
	GetByRegistrationNumber(ctx context.Context, orgID uuid.UUID, registrationNumber string) (*TaxRegistration, error)
}
