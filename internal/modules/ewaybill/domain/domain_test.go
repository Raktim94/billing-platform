package domain

import "testing"

func TestStatus_Terminal(t *testing.T) {
	terminal := []Status{StatusGenerated, StatusCancelled, StatusClosed, StatusFailedFinal}
	nonTerminal := []Status{StatusDraft, StatusQueued, StatusSubmitting, StatusFailedRetryable, StatusCancelPending}

	for _, s := range terminal {
		if !s.Terminal() {
			t.Errorf("%s.Terminal() = false, want true", s)
		}
	}
	for _, s := range nonTerminal {
		if s.Terminal() {
			t.Errorf("%s.Terminal() = true, want false", s)
		}
	}
}

// TestClosed_IsDistinctFromCancelled guards the one field distinction the
// 2026-08-01 GSTN advisory (docs/research.md) actually depends on: CLOSED
// means the shipment happened (just no longer in transit); CANCELLED means
// it never happened. A future refactor that merged these into one status
// would silently break that semantic.
func TestClosed_IsDistinctFromCancelled(t *testing.T) {
	if StatusClosed == StatusCancelled {
		t.Fatal("StatusClosed must be a distinct value from StatusCancelled")
	}
}
