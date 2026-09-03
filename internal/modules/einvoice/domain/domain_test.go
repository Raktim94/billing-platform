package domain

import "testing"

func TestStatus_Terminal(t *testing.T) {
	terminal := []Status{StatusGenerated, StatusFailedFinal, StatusCancelled}
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
