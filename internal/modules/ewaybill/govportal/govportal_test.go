package govportal

import (
	"os"
	"strings"
	"testing"
)

func TestGetOfficialEWayBillPortalURL_DefaultsToRealGovernmentDomain(t *testing.T) {
	os.Unsetenv("EWAYBILL_GOVERNMENT_PORTAL_URL")
	got := NewService().GetOfficialEWayBillPortalURL()
	if !strings.Contains(got, "ewaybillgst.gov.in") {
		t.Fatalf("default portal URL = %q, want the real gov.in domain", got)
	}
}

func TestGetOfficialEWayBillPortalURL_OperatorEnvOverride(t *testing.T) {
	t.Setenv("EWAYBILL_GOVERNMENT_PORTAL_URL", "https://ewaybillgst.gov.in/staging/")
	got := NewService().GetOfficialEWayBillPortalURL()
	if got != "https://ewaybillgst.gov.in/staging/" {
		t.Fatalf("got %q, want the env override to take effect", got)
	}
}
