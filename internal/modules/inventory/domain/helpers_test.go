package domain

import (
	"testing"

	"github.com/shopspring/decimal"
)

func mustDecimal(t *testing.T, s string) decimal.Decimal {
	t.Helper()
	d, err := decimal.NewFromString(s)
	if err != nil {
		t.Fatalf("invalid decimal literal %q: %v", s, err)
	}
	return d
}
