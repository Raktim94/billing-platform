package domain

import "testing"

func TestWeightedAverageCostingStrategy_OnReceipt(t *testing.T) {
	s := WeightedAverageCostingStrategy{}

	tests := []struct {
		name                                      string
		currentQty, currentAvgCost, qty, unitCost string
		want                                      string
	}{
		{
			name:       "first ever receipt, no prior stock",
			currentQty: "0", currentAvgCost: "0", qty: "10", unitCost: "100",
			want: "100",
		},
		{
			name: "blended average, equal quantities",
			// 10 units @ 100 already on hand, receive 10 more @ 200
			// -> (10*100 + 10*200) / 20 = 150
			currentQty: "10", currentAvgCost: "100", qty: "10", unitCost: "200",
			want: "150",
		},
		{
			name:       "receiving at the same cost leaves the average unchanged",
			currentQty: "50", currentAvgCost: "42.5", qty: "25", unitCost: "42.5",
			want: "42.5",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := s.OnReceipt(
				mustDecimal(t, tc.currentQty), mustDecimal(t, tc.currentAvgCost),
				mustDecimal(t, tc.qty), mustDecimal(t, tc.unitCost),
			)
			want := mustDecimal(t, tc.want)
			if !got.Equal(want) {
				t.Fatalf("OnReceipt(%s,%s,%s,%s) = %s, want %s",
					tc.currentQty, tc.currentAvgCost, tc.qty, tc.unitCost, got, want)
			}
		})
	}
}

// TestWeightedAverageCostingStrategy_UnequalQuantitiesWeightCorrectly
// checks a case where the two quantities being blended are very
// different sizes (100 units @ 10 vs. 1 unit @ 1000), so a bug that
// averaged the two unit costs directly (ignoring quantity as a weight)
// would produce 505 instead of the correct ~19.80. The exact result
// (2000/101) has a non-terminating decimal expansion, so this asserts
// against the value rounded to the same 6 decimal places stock_balances
// stores (NUMERIC(20,6)), rather than a hand-computed long-precision
// literal that would be easy to get subtly wrong.
func TestWeightedAverageCostingStrategy_UnequalQuantitiesWeightCorrectly(t *testing.T) {
	s := WeightedAverageCostingStrategy{}
	got := s.OnReceipt(mustDecimal(t, "100"), mustDecimal(t, "10"), mustDecimal(t, "1"), mustDecimal(t, "1000"))
	gotRounded := got.Round(6)
	want := mustDecimal(t, "19.801980")
	if !gotRounded.Equal(want) {
		t.Fatalf("OnReceipt(100,10,1,1000) rounded to 6dp = %s, want %s (raw = %s)", gotRounded, want, got)
	}
	// Sanity bound: the blended average must land strictly between the
	// two input costs (10 and 1000) — this alone catches the
	// "averaged the unit costs, ignored quantity" bug class even without
	// trusting the exact rounded figure above.
	if got.LessThanOrEqual(mustDecimal(t, "10")) || got.GreaterThanOrEqual(mustDecimal(t, "1000")) {
		t.Fatalf("OnReceipt(100,10,1,1000) = %s, expected strictly between 10 and 1000", got)
	}
}

func TestWeightedAverageCostingStrategy_ZeroTotalQuantity(t *testing.T) {
	s := WeightedAverageCostingStrategy{}
	got := s.OnReceipt(mustDecimal(t, "0"), mustDecimal(t, "0"), mustDecimal(t, "0"), mustDecimal(t, "0"))
	if !got.IsZero() {
		t.Fatalf("OnReceipt with zero total quantity = %s, want 0", got)
	}
}
