// Package portal defines the FREE_PORTAL mode's export boundary
// (docs/architecture.md §9b's EWayBillPortalExporter). Concrete mappers
// live in versioned subpackages (portal/v1, a future portal/vNext) —
// exactly the same versioning discipline Stage 8 applied to
// einvoice/v1: a government/portal format change is a new package, cut
// over deliberately, never a silent field patch to a shared struct.
package portal

import (
	"context"

	"rechvix/internal/modules/ewaybill/canonical"
)

// PreparedFile is one output file ready to hand to the user — filename is
// always human-recognizable (docs/architecture.md §9b), never an opaque
// hash.
type PreparedFile struct {
	FileName string
	Content  []byte
}

// Exporter maps a CanonicalEWayBill to the shape the official government
// portal's bulk-upload feature accepts. PrepareUpload never touches a
// database or makes a network call — it's a pure transform from the
// already-captured canonical snapshot to bytes.
type Exporter interface {
	PrepareUpload(ctx context.Context, bill canonical.CanonicalEWayBill) (PreparedFile, error)
	// SchemaVersion identifies which portal format version this Exporter
	// implements (ewaybill_portal_schema_versions.version) — surfaced so
	// callers/audit records can record exactly which mapper version
	// produced a given file.
	SchemaVersion() string
}
