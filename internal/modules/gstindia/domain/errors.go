package domain

import "errors"

var (
	ErrNotFound = errors.New("gstindia: not found")
	// ErrRateNotConfigured is returned by the engine when no tax_rate_master
	// row covers a line's HSN/SAC code on the document date — this is a
	// hard error, never a silent 0%/exempt fallback (brief Rule 2: never
	// invent a tax rule; the brief also requires exempt/nil-rated to be an
	// explicit, configured classification, not an absence of data).
	ErrRateNotConfigured = errors.New("gstindia: no tax rate configured for this HSN/SAC code on this date")
	ErrInvalidGSTIN      = errors.New("gstindia: invalid GSTIN format")
	ErrUnknownStateCode  = errors.New("gstindia: unknown GST state code")
)
