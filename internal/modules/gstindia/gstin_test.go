package gstindia

import "testing"

func TestStateCodeFromGSTIN(t *testing.T) {
	cases := []struct {
		gstin   string
		want    string
		wantErr bool
	}{
		{"27AAAPL1234C1Z5", "27", false}, // Maharashtra
		{"29AAAPL1234C1Z5", "29", false}, // Karnataka
		{"07AAAPL1234C1Z5", "07", false}, // Delhi
		{"", "", true},
		{"27AAAPL1234C1Z", "", true},  // too short
		{"XXAAAPL1234C1Z5", "", true}, // non-digit state code
		{"27aaapl1234c1z5", "", true}, // lowercase not accepted (structural check)
	}
	for _, c := range cases {
		got, err := StateCodeFromGSTIN(c.gstin)
		if c.wantErr {
			if err == nil {
				t.Errorf("StateCodeFromGSTIN(%q): expected error, got %q", c.gstin, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("StateCodeFromGSTIN(%q): unexpected error: %v", c.gstin, err)
			continue
		}
		if got != c.want {
			t.Errorf("StateCodeFromGSTIN(%q) = %q, want %q", c.gstin, got, c.want)
		}
	}
}
