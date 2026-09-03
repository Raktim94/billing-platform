package domain

import "errors"

var (
	ErrNotFound = errors.New("taxation: not found")
	// ErrNoLines is returned when a TaxCalculationInput has zero lines —
	// calculating tax for an empty document is almost certainly a caller
	// bug, not a valid zero-tax result.
	ErrNoLines = errors.New("taxation: at least one line is required")
	// ErrRateNotConfigured is the regime-agnostic sentinel a TaxEngine
	// implementation wraps its own not-configured error with (e.g.
	// gstindia.ErrRateNotConfigured), so callers above taxation in the
	// module layering (sales, purchases, ...) can recognize this specific,
	// user-actionable case via errors.Is without importing a regime plugin
	// directly — see docs/architecture.md §2's layering rule.
	ErrRateNotConfigured = errors.New("taxation: no tax rate configured for this line's HSN/SAC code on this date")
)
