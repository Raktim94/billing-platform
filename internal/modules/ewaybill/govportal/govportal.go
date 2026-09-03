// Package govportal is the ONE place the official government e-Way Bill
// portal's URL is allowed to live (docs/architecture.md §9b — "the portal
// URL is backend-configured and allowlisted, never a user-editable
// arbitrary URL... prevents phishing via a spoofed 'government portal'
// link"). No organisation setting, no database column, no API parameter
// anywhere in this codebase may override this — that is the entire point.
package govportal

import "os"

// defaultURL is the real, official e-Way Bill system portal.
const defaultURL = "https://ewaybillgst.gov.in/"

// Service resolves the portal URL from a fixed, non-tenant-configurable
// source: this constant, or an operator-set environment variable for
// deployments that need to point at a different officially-designated
// mirror/environment — never a per-organisation or per-request value.
type Service struct {
	url string
}

func NewService() *Service {
	url := os.Getenv("EWAYBILL_GOVERNMENT_PORTAL_URL")
	if url == "" {
		url = defaultURL
	}
	return &Service{url: url}
}

// GetOfficialEWayBillPortalURL implements docs/architecture.md §9b's
// GovernmentPortalService.GetOfficialEWayBillPortalURL(). No parameters —
// deliberately: there is nothing for a caller to supply that could steer
// this to a different destination.
func (s *Service) GetOfficialEWayBillPortalURL() string {
	return s.url
}
