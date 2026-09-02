// Package money provides an arbitrary-precision monetary type. It is the
// only place in this codebase permitted to know how a currency amount is
// represented in memory. No other package may store or compute a monetary
// value using float32/float64 (brief §6, §56, Rule 3).
package money

import (
	"fmt"

	"github.com/shopspring/decimal"
)

// RoundingMode controls how a Money value is rounded to its currency's
// minor-unit precision. Different jurisdictions and different calculation
// steps (line tax vs. invoice round-off) may require different modes, so
// this is explicit at every call site rather than a single global default.
type RoundingMode int

const (
	// RoundHalfUp rounds 0.5 away from zero. This is the common statutory
	// default for GST invoice round-off.
	RoundHalfUp RoundingMode = iota
	// RoundHalfEven ("banker's rounding") rounds 0.5 to the nearest even
	// digit, reducing cumulative bias across many roundings.
	RoundHalfEven
	// RoundDown truncates toward zero.
	RoundDown
)

// MinorUnits reports how many digits appear after the decimal separator for
// a currency's smallest unit, per ISO 4217. This table intentionally covers
// only the currencies this project currently has reason to support; unknown
// codes are rejected rather than silently defaulted, because guessing wrong
// here corrupts money.
var minorUnits = map[string]int32{
	"INR": 2,
	"USD": 2,
	"EUR": 2,
	"GBP": 2,
	"AED": 2,
	"SGD": 2,
	"AUD": 2,
	"CAD": 2,
	"JPY": 0,
	"KWD": 3,
	"BHD": 3,
	"OMR": 3,
}

// RegisterCurrency adds or overrides a currency's minor-unit precision.
// Intended for startup-time configuration only (e.g. loading an extended
// ISO 4217 table from settings), not for per-request use.
func RegisterCurrency(code string, minorDigits int32) {
	minorUnits[code] = minorDigits
}

// MinorDigits returns the number of minor-unit digits for a currency code,
// and false if the currency is not registered.
func MinorDigits(currencyCode string) (int32, bool) {
	d, ok := minorUnits[currencyCode]
	return d, ok
}

// Money is a decimal amount tied to an ISO 4217 currency code. The zero
// value is not meaningful; construct via New, Zero, or Parse.
type Money struct {
	amount   decimal.Decimal
	currency string
}

// New builds a Money value from an existing decimal.Decimal. The currency
// code must be registered (see RegisterCurrency); this guards against a
// typo'd currency code silently producing an unrounded, unrecognized value.
func New(amount decimal.Decimal, currencyCode string) (Money, error) {
	if _, ok := minorUnits[currencyCode]; !ok {
		return Money{}, fmt.Errorf("money: unregistered currency code %q", currencyCode)
	}
	return Money{amount: amount, currency: currencyCode}, nil
}

// MustNew is New but panics on error. Intended for startup-time constants
// and tests, never for handling user/request input.
func MustNew(amount decimal.Decimal, currencyCode string) Money {
	m, err := New(amount, currencyCode)
	if err != nil {
		panic(err)
	}
	return m
}

// Zero returns a zero-value Money in the given currency.
func Zero(currencyCode string) (Money, error) {
	return New(decimal.Zero, currencyCode)
}

// Parse builds a Money value from a decimal string (e.g. "76.271186"). Use
// this at system boundaries (API request bodies, CSV import) instead of any
// float-based parsing.
func Parse(amountStr, currencyCode string) (Money, error) {
	d, err := decimal.NewFromString(amountStr)
	if err != nil {
		return Money{}, fmt.Errorf("money: invalid amount %q: %w", amountStr, err)
	}
	return New(d, currencyCode)
}

// Currency returns the ISO 4217 currency code.
func (m Money) Currency() string { return m.currency }

// Decimal returns the underlying full-precision decimal amount. Callers
// needing a rounded, currency-correct value must call Round explicitly —
// this accessor intentionally does not round, so that intermediate
// calculations (e.g. inside the tax engine) are never silently truncated
// before the documented rounding point (brief §6: "Do NOT round
// intermediate values prematurely").
func (m Money) Decimal() decimal.Decimal { return m.amount }

func (m Money) sameCurrency(other Money) error {
	if m.currency != other.currency {
		return fmt.Errorf("money: currency mismatch: %s vs %s", m.currency, other.currency)
	}
	return nil
}

// Add returns m + other. Both operands must share a currency; mixing
// currencies without an explicit exchange-rate conversion is a bug, not a
// value to silently coerce.
func (m Money) Add(other Money) (Money, error) {
	if err := m.sameCurrency(other); err != nil {
		return Money{}, err
	}
	return Money{amount: m.amount.Add(other.amount), currency: m.currency}, nil
}

// Sub returns m - other. See Add for the currency-match requirement.
func (m Money) Sub(other Money) (Money, error) {
	if err := m.sameCurrency(other); err != nil {
		return Money{}, err
	}
	return Money{amount: m.amount.Sub(other.amount), currency: m.currency}, nil
}

// MulDecimal multiplies the amount by an arbitrary-precision factor (e.g. a
// quantity, a tax rate expressed as a fraction, an exchange rate). The
// result retains full precision — round explicitly at the point the
// business rule designates as final.
func (m Money) MulDecimal(factor decimal.Decimal) Money {
	return Money{amount: m.amount.Mul(factor), currency: m.currency}
}

// Neg returns the additive inverse.
func (m Money) Neg() Money {
	return Money{amount: m.amount.Neg(), currency: m.currency}
}

// IsZero reports whether the amount is exactly zero.
func (m Money) IsZero() bool { return m.amount.IsZero() }

// IsNegative reports whether the amount is less than zero.
func (m Money) IsNegative() bool { return m.amount.IsNegative() }

// Cmp compares m to other, returning -1, 0, or 1. Both operands must share
// a currency.
func (m Money) Cmp(other Money) (int, error) {
	if err := m.sameCurrency(other); err != nil {
		return 0, err
	}
	return m.amount.Cmp(other.amount), nil
}

// Round rounds the amount to the currency's registered minor-unit
// precision using the given rounding mode. This is the only place a
// monetary value should lose precision; call it exactly once, at the point
// a business rule (statutory round-off, invoice line total, ledger
// posting) designates as final.
func (m Money) Round(mode RoundingMode) Money {
	digits, ok := minorUnits[m.currency]
	if !ok {
		// Constructed via New/Parse, which already validated the currency,
		// so this should be unreachable outside a bug in this package.
		panic(fmt.Sprintf("money: currency %q lost its minor-unit registration", m.currency))
	}
	var rounded decimal.Decimal
	switch mode {
	case RoundHalfEven:
		rounded = m.amount.RoundBank(digits)
	case RoundDown:
		rounded = m.amount.Truncate(digits)
	default: // RoundHalfUp
		rounded = m.amount.Round(digits)
	}
	return Money{amount: rounded, currency: m.currency}
}

// String renders the amount at full stored precision (not currency-rounded)
// followed by the currency code, e.g. "76.271186186 INR". Callers wanting a
// display value should Round first.
func (m Money) String() string {
	return fmt.Sprintf("%s %s", m.amount.String(), m.currency)
}

// StringFixed renders the amount rounded to the currency's minor-unit
// precision, e.g. "76.27". Suitable for receipts, PDFs, and API responses
// that must show a final, statutory-rounded figure.
func (m Money) StringFixed(mode RoundingMode) string {
	return m.Round(mode).amount.StringFixed(minorUnits[m.currency])
}
