// Package domain holds India GST-specific master data types and repository
// interfaces (docs/architecture.md §8): tax_rate_master (which rate
// applies to an HSN/SAC code on a given date) and gst_state_codes (state/UT
// reference data, modeled as data rather than a Go enum per
// docs/research.md — the list has changed before). No I/O, no framework
// imports.
package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type RateClassification string

const (
	ClassificationTaxable   RateClassification = "TAXABLE"
	ClassificationExempt    RateClassification = "EXEMPT"
	ClassificationNilRated  RateClassification = "NIL_RATED"
	ClassificationZeroRated RateClassification = "ZERO_RATED"
)

// TaxRateMaster is one (organisation, HSN/SAC code, validity window) rate
// row. GSTRate is the COMBINED rate (e.g. 18 for "18% GST") — the engine
// splits it into CGST+SGST/UTGST or IGST at calculation time; this table
// never stores a pre-split pair (brief §18).
type TaxRateMaster struct {
	ID             uuid.UUID
	OrganisationID uuid.UUID
	CountryCode    string
	HSNSACCode     string
	Classification RateClassification
	GSTRate        decimal.Decimal
	CessRate       decimal.Decimal
	ValidFrom      time.Time
	ValidTo        *time.Time
	CreatedAt      time.Time
}

// CoversDate reports whether asOf falls within [ValidFrom, ValidTo]
// (ValidTo of nil meaning "still open").
func (r TaxRateMaster) CoversDate(asOf time.Time) bool {
	if asOf.Before(r.ValidFrom) {
		return false
	}
	return r.ValidTo == nil || !asOf.After(*r.ValidTo)
}

// GSTState is one row of India's GST state/UT code table — global
// reference data, not organisation-scoped (see migrations/0015_gstindia.up.sql).
type GSTState struct {
	Code             string
	Name             string
	IsUnionTerritory bool
}

// --- Repository interfaces ---

type TaxRateRepository interface {
	Create(ctx context.Context, r *TaxRateMaster) error
	// Resolve returns the rate row for (orgID, countryCode, hsnSacCode)
	// that covers asOf — the most recent row whose validity window
	// contains asOf. Returns domain.ErrNotFound if none exists, which the
	// engine treats as a hard configuration error (never a silent 0%
	// default — brief Rule 2: never invent a tax rule).
	Resolve(ctx context.Context, orgID uuid.UUID, countryCode, hsnSacCode string, asOf time.Time) (*TaxRateMaster, error)
	ListByHSN(ctx context.Context, orgID uuid.UUID, countryCode, hsnSacCode string) ([]*TaxRateMaster, error)
}

type StateRepository interface {
	// GetByCode returns the state/UT row for a GST state code, or
	// domain.ErrNotFound if the code isn't in gst_state_codes.
	GetByCode(ctx context.Context, code string) (*GSTState, error)
}
