package outbox

import (
	"errors"
	"testing"
	"time"
)

func TestBackoff_GrowsAndCaps(t *testing.T) {
	cases := []struct {
		attempts int
		want     time.Duration
	}{
		{1, time.Minute},
		{2, 2 * time.Minute},
		{3, 4 * time.Minute},
		{4, 8 * time.Minute},
		{10, time.Hour}, // caps at 1h well before attempt 10
	}
	for _, c := range cases {
		if got := backoff(c.attempts); got != c.want {
			t.Errorf("backoff(%d) = %v, want %v", c.attempts, got, c.want)
		}
	}
}

func TestPermanent_MarksErrorNonRetryable(t *testing.T) {
	base := errors.New("malformed request")
	wrapped := Permanent(base)

	if !isPermanent(wrapped) {
		t.Fatal("expected Permanent(err) to be detected as permanent")
	}
	if isPermanent(base) {
		t.Fatal("expected the unwrapped base error to NOT be permanent")
	}
	if !errors.Is(wrapped, wrapped) {
		t.Fatal("sanity: errors.Is should match itself")
	}
	if wrapped.Error() != base.Error() {
		t.Fatalf("Permanent(err).Error() = %q, want %q (message must be preserved)", wrapped.Error(), base.Error())
	}
}

func TestPermanent_Nil(t *testing.T) {
	if Permanent(nil) != nil {
		t.Fatal("Permanent(nil) must return nil, not a wrapped nil")
	}
}
