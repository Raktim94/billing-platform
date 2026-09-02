package money

import (
	"encoding/json"
	"testing"

	"github.com/shopspring/decimal"
)

func TestParseAndRound(t *testing.T) {
	m, err := Parse("76.2711864406779661", "INR")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := m.StringFixed(RoundHalfUp); got != "76.27" {
		t.Errorf("StringFixed(RoundHalfUp) = %q, want 76.27", got)
	}
}

func TestParseUnregisteredCurrency(t *testing.T) {
	if _, err := Parse("10", "ZZZ"); err == nil {
		t.Fatal("expected error for unregistered currency, got nil")
	}
}

func TestAddRequiresSameCurrency(t *testing.T) {
	a := MustNew(decimal.NewFromInt(10), "INR")
	b := MustNew(decimal.NewFromInt(5), "USD")
	if _, err := a.Add(b); err == nil {
		t.Fatal("expected currency mismatch error, got nil")
	}
}

func TestAddSub(t *testing.T) {
	a := MustNew(decimal.NewFromFloat(90), "INR")
	b := MustNew(decimal.NewFromFloat(13.728814), "INR")
	sum, err := a.Add(b)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if got := sum.StringFixed(RoundHalfUp); got != "103.73" {
		t.Errorf("sum = %q, want 103.73", got)
	}

	diff, err := a.Sub(b)
	if err != nil {
		t.Fatalf("Sub: %v", err)
	}
	if got := diff.StringFixed(RoundHalfUp); got != "76.27" {
		t.Errorf("diff = %q, want 76.27", got)
	}
}

func TestMulDecimalPreservesPrecision(t *testing.T) {
	// 90 / 1.18 = 76.271186440677966101...  Verifies MulDecimal/division
	// style composition never truncates before an explicit Round call.
	gross := MustNew(decimal.NewFromInt(90), "INR")
	divisor := decimal.NewFromFloat(1.18)
	taxable := gross.MulDecimal(decimal.NewFromInt(1).Div(divisor))

	if taxable.Round(RoundHalfUp).StringFixed(RoundHalfUp) != "76.27" {
		t.Errorf("rounded taxable = %s, want 76.27", taxable.StringFixed(RoundHalfUp))
	}
	// The unrounded decimal must retain more than 2 digits of precision.
	if taxable.Decimal().Exponent() >= -2 {
		t.Errorf("expected full precision before rounding, got exponent %d", taxable.Decimal().Exponent())
	}
}

func TestRoundingModes(t *testing.T) {
	half := MustNew(decimal.NewFromFloat(10.125), "INR")

	if got := half.StringFixed(RoundHalfUp); got != "10.13" {
		t.Errorf("RoundHalfUp: got %q, want 10.13", got)
	}
	if got := half.StringFixed(RoundDown); got != "10.12" {
		t.Errorf("RoundDown: got %q, want 10.12", got)
	}
	// 10.125 rounds to even (10.12) under banker's rounding at 2 digits.
	if got := half.StringFixed(RoundHalfEven); got != "10.12" {
		t.Errorf("RoundHalfEven: got %q, want 10.12", got)
	}
}

func TestZeroMinorDigitsCurrency(t *testing.T) {
	m := MustNew(decimal.NewFromFloat(1234.7), "JPY")
	if got := m.StringFixed(RoundHalfUp); got != "1235" {
		t.Errorf("JPY StringFixed = %q, want 1235 (0 minor digits)", got)
	}
}

func TestCmp(t *testing.T) {
	a := MustNew(decimal.NewFromInt(10), "INR")
	b := MustNew(decimal.NewFromInt(20), "INR")
	c, err := a.Cmp(b)
	if err != nil {
		t.Fatalf("Cmp: %v", err)
	}
	if c != -1 {
		t.Errorf("Cmp(10,20) = %d, want -1", c)
	}
}

func TestNegIsZeroIsNegative(t *testing.T) {
	a := MustNew(decimal.NewFromInt(5), "INR")
	neg := a.Neg()
	if !neg.IsNegative() {
		t.Error("expected Neg() to be negative")
	}
	zero, _ := Zero("INR")
	if !zero.IsZero() {
		t.Error("expected Zero() to be zero")
	}
}

func TestRegisterCurrency(t *testing.T) {
	RegisterCurrency("XTS", 4)
	digits, ok := MinorDigits("XTS")
	if !ok || digits != 4 {
		t.Errorf("RegisterCurrency did not take effect: digits=%d ok=%v", digits, ok)
	}
}

func TestMoney_JSONRoundTrip(t *testing.T) {
	original := MustNew(decimal.RequireFromString("76.271186"), "INR")

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded Money
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if decoded.Currency() != original.Currency() {
		t.Errorf("currency: got %q, want %q", decoded.Currency(), original.Currency())
	}
	if !decoded.Decimal().Equal(original.Decimal()) {
		t.Errorf("amount: got %s, want %s (full precision must survive the round trip, not just the rounded display value)", decoded.Decimal(), original.Decimal())
	}
}

func TestMoney_JSON_PreservesFullPrecision_NotJustDisplayRounding(t *testing.T) {
	// A price list item's underlying price can carry more precision than
	// the currency's display minor units (e.g. computed unit costs) —
	// marshaling must not silently round it away, since Round() is only
	// ever meant to happen at one documented, explicit call site (brief §6).
	m := MustNew(decimal.RequireFromString("12.3456789"), "INR")
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if string(data) != `{"amount":"12.3456789","currency":"INR"}` {
		t.Errorf("Marshal produced %s, want full-precision amount, not currency-rounded", data)
	}
}

func TestMoney_UnmarshalJSON_RejectsUnregisteredCurrency(t *testing.T) {
	var m Money
	err := json.Unmarshal([]byte(`{"amount":"10.00","currency":"ZZZ"}`), &m)
	if err == nil {
		t.Fatal("expected an error unmarshaling an unregistered currency code")
	}
}
