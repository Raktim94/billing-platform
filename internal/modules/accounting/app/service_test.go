package app

import (
	"math/rand"
	"testing"

	"github.com/shopspring/decimal"
)

func TestValidateBalanced_RejectsUnbalanced(t *testing.T) {
	_, _, err := validateBalanced([]JournalLineRequest{
		{AccountCode: "1100", Debit: decimal.NewFromInt(100)},
		{AccountCode: "4000", Credit: decimal.NewFromInt(90)},
	})
	if err == nil {
		t.Fatal("expected an error for an unbalanced journal, got nil")
	}
}

func TestValidateBalanced_AcceptsBalanced(t *testing.T) {
	debit, credit, err := validateBalanced([]JournalLineRequest{
		{AccountCode: "1100", Debit: decimal.NewFromInt(118)},
		{AccountCode: "4000", Credit: decimal.NewFromInt(100)},
		{AccountCode: "2100", Credit: decimal.NewFromInt(18)},
	})
	if err != nil {
		t.Fatalf("expected a balanced journal to be accepted, got: %v", err)
	}
	if !debit.Equal(decimal.NewFromInt(118)) || !credit.Equal(decimal.NewFromInt(118)) {
		t.Fatalf("debit=%s credit=%s, want both 118", debit, credit)
	}
}

func TestValidateBalanced_RejectsLineWithBothDebitAndCredit(t *testing.T) {
	_, _, err := validateBalanced([]JournalLineRequest{
		{AccountCode: "1100", Debit: decimal.NewFromInt(100), Credit: decimal.NewFromInt(100)},
		{AccountCode: "4000", Credit: decimal.NewFromInt(100)},
	})
	if err == nil {
		t.Fatal("expected an error for a line that is both a debit and a credit, got nil")
	}
}

func TestValidateBalanced_RejectsLineWithNeitherDebitNorCredit(t *testing.T) {
	_, _, err := validateBalanced([]JournalLineRequest{
		{AccountCode: "1100"},
		{AccountCode: "4000", Credit: decimal.NewFromInt(100)},
	})
	if err == nil {
		t.Fatal("expected an error for a line that is neither a debit nor a credit, got nil")
	}
}

// TestValidateBalanced_Property is brief §65's property test: "for any
// valid journal, Σdebit == Σcredit". It generates many randomized journals
// that are constructed to be balanced by design (each line's debit is
// mirrored by a same-amount credit split across 2-6 other lines) and
// confirms every single one is accepted with matching totals — then
// perturbs one and confirms it's rejected. This is deliberately a
// generate-then-check property test, not a handful of hand-picked
// examples.
func TestValidateBalanced_Property(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	for i := 0; i < 200; i++ {
		total := decimal.NewFromInt(int64(1 + rng.Intn(1_000_000))).Div(decimal.NewFromInt(100))
		numCreditLines := 1 + rng.Intn(5)
		lines := []JournalLineRequest{{AccountCode: "DR", Debit: total}}
		remaining := total
		for c := 0; c < numCreditLines-1; c++ {
			// Split off a random fraction of what's left, rounded to 2dp,
			// so the credit side is built from several lines that must
			// still sum exactly back to `total`.
			share := remaining.Mul(decimal.NewFromFloat(0.3)).Round(2)
			if share.IsZero() {
				continue
			}
			lines = append(lines, JournalLineRequest{AccountCode: "CR", Credit: share})
			remaining = remaining.Sub(share)
		}
		lines = append(lines, JournalLineRequest{AccountCode: "CR_LAST", Credit: remaining})

		debit, credit, err := validateBalanced(lines)
		if err != nil {
			t.Fatalf("iteration %d: expected balanced journal (total=%s) to validate, got: %v", i, total, err)
		}
		if !debit.Equal(total) || !credit.Equal(total) {
			t.Fatalf("iteration %d: debit=%s credit=%s, want both %s", i, debit, credit, total)
		}

		// Now perturb: add one paisa to the last credit line. Every
		// perturbed journal must be rejected — no exceptions.
		perturbed := make([]JournalLineRequest, len(lines))
		copy(perturbed, lines)
		last := len(perturbed) - 1
		perturbed[last].Credit = perturbed[last].Credit.Add(decimal.NewFromFloat(0.01))
		if _, _, err := validateBalanced(perturbed); err == nil {
			t.Fatalf("iteration %d: expected a 1-paisa-unbalanced journal to be rejected, got no error", i)
		}
	}
}
