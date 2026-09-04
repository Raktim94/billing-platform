//go:build integration

package integration

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"rechvix/internal/platform/outbox"
)

// processNextForOrg is a TEST-ONLY alternative to outbox.Poller.ProcessOnce
// for integration tests that need to process one outbox event
// deterministically. It exists because of a real, pre-existing
// architectural gap this Stage 9 work exposed: the whole integration test
// BINARY shares one physical outbox_events table for its entire run (one
// Postgres container, started once in TestMain) with no per-test
// partitioning, and outbox.Poller.ProcessOnce claims the globally oldest
// claimable row via outbox_claim_next() — a SECURITY DEFINER function that
// deliberately bypasses RLS (it has to, to see every organisation's rows
// before knowing which one to scope to; see migrations/0023's comment).
// Any test whose poller has a handler for an event type another test also
// produces can end up claiming and fully processing THAT OTHER test's
// event — not just wasting a claim slot, but actually running real
// business logic (e.g. calling a mock provider) against a foreign
// document, corrupting call-count assertions in both tests. This was
// already possible in Stage 8 with a single event type per finalize; Stage
// 9 (a second event type per finalize, plus two new outbox-consuming test
// files) made it manifest reliably rather than being a latent risk.
//
// The correct fix — used here — is to claim scoped to ONE organisation,
// via a normal RLS-protected query inside RunScoped(ctx, orgID, ...),
// instead of the cross-organisation SECURITY DEFINER path. Since every
// test already creates a fresh, unique organisation
// (bootstrapTestTenant/setupSalesFixture), this makes cross-test
// contamination structurally impossible for any test that uses it: RLS
// itself — the same mechanism already proven in tests/integration/rls_test.go
// — guarantees a query scoped to org A can never see org B's rows.
//
// Not a production code change: this reimplements the same UPDATE...
// RETURNING / claim / mark-done-or-failed shape as
// internal/platform/outbox.PGStore, deliberately kept in the test package
// rather than added as a second production entry point on PGStore, since
// "claim scoped to exactly one already-known organisation" is a test-only
// need — production code (apps/worker) always needs the cross-organisation
// claim, precisely because a real worker doesn't know in advance which
// organisation's event is next.
func processNextForOrg(t *testing.T, ctx context.Context, orgID uuid.UUID, handlers map[string]outbox.Handler) bool {
	t.Helper()
	found := false
	err := sharedPool.RunScoped(ctx, orgID, func(ctx context.Context) error {
		const claimQ = `
			UPDATE outbox_events
			SET status = 'PROCESSING', attempts = attempts + 1, updated_at = now()
			WHERE id = (
				SELECT id FROM outbox_events
				WHERE organisation_id = $1 AND status IN ('PENDING', 'FAILED') AND next_attempt_at <= now()
				ORDER BY created_at
				FOR UPDATE SKIP LOCKED
				LIMIT 1
			)
			RETURNING id, organisation_id, event_type, payload, idempotency_key, status, attempts, next_attempt_at, last_error, created_at, updated_at`
		row := sharedPool.Q(ctx).QueryRow(ctx, claimQ, orgID)
		var ev outbox.Event
		var payload []byte
		if err := row.Scan(&ev.ID, &ev.OrganisationID, &ev.EventType, &payload, &ev.IdempotencyKey,
			&ev.Status, &ev.Attempts, &ev.NextAttemptAt, &ev.LastError, &ev.CreatedAt, &ev.UpdatedAt); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil // nothing claimable for this org right now
			}
			return fmt.Errorf("claiming next event for org %s: %w", orgID, err)
		}
		ev.Payload = payload
		found = true

		handler, ok := handlers[ev.EventType]
		var handleErr error
		if !ok {
			handleErr = fmt.Errorf("processNextForOrg: no handler registered for event type %q", ev.EventType)
		} else {
			handleErr = handler(ctx, ev)
		}

		if handleErr != nil {
			_, err := sharedPool.Q(ctx).Exec(ctx,
				`UPDATE outbox_events SET status = 'FAILED', last_error = $2, next_attempt_at = now() + interval '1 hour', updated_at = now() WHERE id = $1`,
				ev.ID, handleErr.Error())
			return err
		}
		_, err := sharedPool.Q(ctx).Exec(ctx,
			`UPDATE outbox_events SET status = 'DONE', updated_at = now() WHERE id = $1`, ev.ID)
		return err
	})
	if err != nil {
		t.Fatalf("processNextForOrg: %v", err)
	}
	return found
}

// drainForOrg calls processNextForOrg repeatedly for orgID until nothing
// more is claimable, up to maxAttempts as a safety bound. Since it's
// scoped to a single test's own organisation, "drain everything" is
// always safe here — there is nothing else that could ever be queued
// under that organisation.
func drainForOrg(t *testing.T, ctx context.Context, orgID uuid.UUID, handlers map[string]outbox.Handler, maxAttempts int) (drained int) {
	t.Helper()
	for i := 0; i < maxAttempts; i++ {
		if !processNextForOrg(t, ctx, orgID, handlers) {
			return drained
		}
		drained++
	}
	return drained
}
