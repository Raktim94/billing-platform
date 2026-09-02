package numbering

import (
	"testing"
	"time"
)

func TestFinancialYearFor(t *testing.T) {
	cases := []struct {
		date time.Time
		want string
	}{
		{time.Date(2026, time.January, 15, 0, 0, 0, 0, time.UTC), "2025-26"},
		{time.Date(2026, time.March, 31, 0, 0, 0, 0, time.UTC), "2025-26"},
		{time.Date(2026, time.April, 1, 0, 0, 0, 0, time.UTC), "2026-27"},
		{time.Date(2026, time.June, 15, 0, 0, 0, 0, time.UTC), "2026-27"},
		{time.Date(2027, time.February, 1, 0, 0, 0, 0, time.UTC), "2026-27"},
		{time.Date(2099, time.December, 31, 0, 0, 0, 0, time.UTC), "2099-00"},
	}
	for _, c := range cases {
		if got := FinancialYearFor(c.date); got != c.want {
			t.Errorf("FinancialYearFor(%s) = %q, want %q", c.date.Format("2006-01-02"), got, c.want)
		}
	}
}
