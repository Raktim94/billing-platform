// Package outbox implements the transactional outbox pattern
// (docs/architecture.md §34, brief §12/§33): a caller (e.g.
// sales.FinalizeDocument) writes an Event row inside its own database
// transaction, so the event is queued atomically with whatever business
// change it describes — never lost to a crash between "the sale finalized"
// and "the e-Invoice request was queued," and never queued if the sale
// itself rolled back. A separate process (apps/worker) later claims and
// processes events; it is never inline with the HTTP request path that
// created them (docs/architecture.md §9, brief Rule 12, Scenario L).
//
// This package is deliberately generic (event_type + a jsonb payload), not
// e-Invoice-specific — Stage 8 is its first caller, but Stage 9's
// notification/webhook delivery is expected to reuse the same table rather
// than build a second outbox.
package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"rechvix/internal/platform/database"
)

type Status string

const (
	StatusPending    Status = "PENDING"
	StatusProcessing Status = "PROCESSING"
	StatusDone       Status = "DONE"
	StatusFailed     Status = "FAILED"
)

type Event struct {
	ID             uuid.UUID
	OrganisationID uuid.UUID
	EventType      string
	Payload        []byte
	IdempotencyKey string
	Status         Status
	Attempts       int
	NextAttemptAt  time.Time
	LastError      *string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// Handler processes one claimed Event. Returning a nil error marks the
// event DONE; returning an error marks it FAILED and schedules a retry
// (unless IsPermanent(err) is true, in which case it stays FAILED without
// being retried — a distinction the einvoice/ewaybill callers use for
// FAILED_FINAL-shaped errors like a malformed request that will never
// succeed on retry).
type Handler func(ctx context.Context, event Event) error

// permanentError marks a Handler error as non-retryable. Wrap with
// Permanent(err) rather than constructing this directly.
type permanentError struct{ err error }

func (p *permanentError) Error() string { return p.err.Error() }
func (p *permanentError) Unwrap() error { return p.err }

// Permanent wraps err so the poller does not schedule a retry for it.
func Permanent(err error) error {
	if err == nil {
		return nil
	}
	return &permanentError{err: err}
}

func isPermanent(err error) bool {
	var p *permanentError
	return err != nil && errors.As(err, &p)
}

// Writer enqueues events. Constructed from a *database.Pool but typed as a
// narrow interface so application-layer services (sales.Service) that only
// need to enqueue don't have to depend on the whole outbox package's
// poller/claim surface — mirrors audit.Recorder's shape.
type Writer interface {
	Enqueue(ctx context.Context, orgID uuid.UUID, eventType, idempotencyKey string, payload any) error
}

type PGStore struct {
	pool *database.Pool
}

func NewPGStore(pool *database.Pool) *PGStore {
	return &PGStore{pool: pool}
}

var _ Writer = (*PGStore)(nil)

// Enqueue must be called with ctx carrying the caller's own transaction
// (i.e. from inside a pool.RunScoped/pool.Run block) for the "atomic with
// the business change" guarantee to actually hold — it uses
// pool.Q(ctx), which binds to whatever transaction is already active in
// ctx, exactly like every other repository in this codebase.
//
// ON CONFLICT DO NOTHING on (organisation_id, idempotency_key): enqueuing
// the same logical event twice is a silent no-op, not an error — a caller
// that calls Enqueue defensively (e.g. "make sure an e-Invoice request is
// queued for this document") doesn't need to pre-check whether one already
// exists.
func (s *PGStore) Enqueue(ctx context.Context, orgID uuid.UUID, eventType, idempotencyKey string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("outbox: marshaling payload: %w", err)
	}
	id, err := uuid.NewV7()
	if err != nil {
		return fmt.Errorf("outbox: generating id: %w", err)
	}
	const q = `
		INSERT INTO outbox_events (id, organisation_id, event_type, payload, idempotency_key)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (organisation_id, idempotency_key) DO NOTHING`
	if _, err := s.pool.Q(ctx).Exec(ctx, q, id, orgID, eventType, body, idempotencyKey); err != nil {
		return fmt.Errorf("outbox: inserting event: %w", err)
	}
	return nil
}

// ClaimNext calls the outbox_claim_next() SECURITY DEFINER function
// (migrations/0023_outbox.up.sql) — the one deliberate, narrow,
// documented bypass of outbox_events' RLS, needed because the poller has
// no single organisation to scope a normal query to until this call tells
// it one. Must be run OUTSIDE any RunScoped block (a bare pool.Run or
// direct call is fine — see poller.go) since it needs to see every
// organisation's rows, not just whichever scope might already be set.
// Returns (nil, false, nil) when there is nothing to claim.
func (s *PGStore) ClaimNext(ctx context.Context) (*Event, bool, error) {
	row := s.pool.Q(ctx).QueryRow(ctx, `SELECT * FROM outbox_claim_next()`)
	var e Event
	var payload []byte
	err := row.Scan(&e.ID, &e.OrganisationID, &e.EventType, &payload, &e.IdempotencyKey,
		&e.Status, &e.Attempts, &e.NextAttemptAt, &e.LastError, &e.CreatedAt, &e.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		// outbox_claim_next() is RETURNS SETOF: zero result rows means
		// nothing was claimable, not an error.
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("outbox: claiming next event: %w", err)
	}
	e.Payload = payload
	return &e, true, nil
}

// MarkDone and MarkFailed must be called with ctx scoped to the claimed
// event's own organisation (via RunScoped(ctx, event.OrganisationID, ...))
// — from that point on, normal RLS applies, same as every other write in
// this codebase; there is nothing special about the outbox row once it's
// been claimed and its organisation is known.

func (s *PGStore) MarkDone(ctx context.Context, id uuid.UUID) error {
	_, err := s.pool.Q(ctx).Exec(ctx,
		`UPDATE outbox_events SET status = 'DONE', updated_at = now() WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("outbox: marking event done: %w", err)
	}
	return nil
}

// MarkFailed schedules a retry (status FAILED, next_attempt_at pushed out
// by an exponential backoff based on attempts) unless handlerErr is
// Permanent, in which case it's left FAILED with next_attempt_at far in
// the future — still queryable/visible for operator review, but the
// poller's WHERE next_attempt_at <= now() will not pick it up again.
func (s *PGStore) MarkFailed(ctx context.Context, id uuid.UUID, attempts int, handlerErr error) error {
	msg := handlerErr.Error()
	next := time.Now().Add(backoff(attempts))
	if isPermanent(handlerErr) {
		next = time.Now().Add(100 * 365 * 24 * time.Hour) // effectively never
	}
	_, err := s.pool.Q(ctx).Exec(ctx,
		`UPDATE outbox_events SET status = 'FAILED', last_error = $2, next_attempt_at = $3, updated_at = now() WHERE id = $1`,
		id, msg, next)
	if err != nil {
		return fmt.Errorf("outbox: marking event failed: %w", err)
	}
	return nil
}

// backoff is a simple capped exponential curve (1m, 2m, 4m, ... capped at
// 1h) — good enough for a government-API retry cadence; not intended to be
// configurable per event type yet.
func backoff(attempts int) time.Duration {
	d := time.Minute
	for i := 1; i < attempts && d < time.Hour; i++ {
		d *= 2
	}
	if d > time.Hour {
		d = time.Hour
	}
	return d
}
