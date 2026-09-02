package domain

import "errors"

var (
	ErrNotFound = errors.New("taxation: not found")
	// ErrNoLines is returned when a TaxCalculationInput has zero lines —
	// calculating tax for an empty document is almost certainly a caller
	// bug, not a valid zero-tax result.
	ErrNoLines = errors.New("taxation: at least one line is required")
)
