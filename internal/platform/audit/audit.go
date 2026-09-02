// Package audit implements the immutable audit trail required by brief
// §30. Every sensitive mutation (login, password change, role change,
// invoice finalize/cancel, stock adjustment, ...) records one Entry
// through Recorder, in the same database transaction as the mutation
// itself — so an audit record and the change it describes are never out
// of sync (both commit together, or neither does).
package audit

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// ActorType identifies what kind of principal performed an action.
type ActorType string

const (
	ActorUser   ActorType = "USER"
	ActorSystem ActorType = "SYSTEM"
	ActorAPIKey ActorType = "API_KEY"
)

// Entry is one audit record. BeforeState/AfterState are arbitrary
// JSON-serializable snapshots (map[string]any, or a struct that marshals
// cleanly) — callers must never include a password hash, MFA secret, API
// key value, or other raw secret in either (brief §30). This package does
// not attempt to scrub arbitrary structs for secret-shaped fields; that
// responsibility sits with each call site, which knows what its own
// entity's sensitive fields are.
type Entry struct {
	OrganisationID uuid.UUID
	ActorUserID    *uuid.UUID
	ActorType      ActorType
	Action         string
	EntityType     string
	EntityID       *uuid.UUID
	BeforeState    any
	AfterState     any
	IP             string
	UserAgent      string
	RequestID      string
	At             time.Time
}

// Recorder persists an audit Entry. Implemented against Postgres in
// pg.go.
type Recorder interface {
	Record(ctx context.Context, entry Entry) error
}
