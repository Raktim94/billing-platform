package gstindia

import (
	"regexp"

	"billing-platform/internal/modules/gstindia/domain"
)

// gstinPattern is a structural check only — 2-digit state code, 10-char
// PAN, 1-digit entity number, 'Z' literal, 1 checksum char. Deliberately
// NOT verifying the checksum digit itself: a false hard-reject on a real
// customer's GSTIN due to a checksum-algorithm bug is worse than accepting
// a structurally-valid-looking string that a human can still visually
// correct, matching this project's established stance on advisory-only
// GSTIN validation elsewhere (see the sibling nodedr-pos project's
// isValidGstinFormat).
var gstinPattern = regexp.MustCompile(`^[0-9]{2}[A-Z]{5}[0-9]{4}[A-Z]{1}[1-9A-Z]{1}Z[0-9A-Z]{1}$`)

// StateCodeFromGSTIN extracts the two-digit GST state code from a GSTIN's
// first two characters, after a structural format check. Returns
// domain.ErrInvalidGSTIN if the format is wrong.
func StateCodeFromGSTIN(gstin string) (string, error) {
	if !gstinPattern.MatchString(gstin) {
		return "", domain.ErrInvalidGSTIN
	}
	return gstin[:2], nil
}
