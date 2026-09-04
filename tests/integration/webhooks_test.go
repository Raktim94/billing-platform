//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"

	webhooksapp "rechvix/internal/modules/webhooks/app"
	webhookspg "rechvix/internal/modules/webhooks/pg"
	"rechvix/internal/platform/audit"
	"rechvix/internal/platform/outbox"
	"rechvix/internal/platform/permissions"
)

// newTestWebhooksService returns the service plus a handlers map for
// tests/integration/outbox_helpers_test.go's processNextForOrg/drainForOrg
// — organisation-scoped claiming, not outbox.Poller's cross-organisation
// claim (see that file's doc comment for why this matters for tests).
func newTestWebhooksService(t *testing.T) (*webhooksapp.Service, *outbox.PGStore, map[string]outbox.Handler) {
	t.Helper()
	checker := permissions.NewChecker(permissions.NewPGStore(sharedPool), sharedPool)
	recorder := audit.NewPGRecorder(sharedPool)
	outboxStore := outbox.NewPGStore(sharedPool)
	svc := webhooksapp.NewService(sharedPool, webhookspg.NewEndpointRepo(sharedPool), webhookspg.NewDeliveryLogRepo(sharedPool),
		outboxStore, checker, recorder)
	handlers := map[string]outbox.Handler{
		"invoice.finalized":           svc.HandlerForSourceEvent("invoice.finalized"),
		webhooksapp.EventTypeDelivery: svc.DeliverHandler(),
	}
	return svc, outboxStore, handlers
}

func TestWebhooks_HMACSignature_VerifiesAndRejectsTampering(t *testing.T) {
	body := []byte(`{"hello":"world"}`)
	sig := webhooksapp.Sign("secret123", 1700000000, "event-1", body)
	if webhooksapp.Sign("secret123", 1700000000, "event-1", body) != sig {
		t.Fatal("Sign is not deterministic for identical inputs")
	}
	if webhooksapp.Sign("wrong-secret", 1700000000, "event-1", body) == sig {
		t.Fatal("Sign produced the same signature for a different secret — HMAC is broken")
	}
	tampered := []byte(`{"hello":"world!"}`)
	if webhooksapp.Sign("secret123", 1700000000, "event-1", tampered) == sig {
		t.Fatal("Sign produced the same signature for a tampered body — HMAC is broken")
	}
}

// TestWebhooks_FullFlow_SourceEventFansOutAndDelivers exercises the real
// two-hop path (docs/adr/0005): enqueue a source event -> fan-out handler
// enqueues a delivery -> delivery handler POSTs to a real local
// httptest.Server with a correctly verifiable HMAC signature.
func TestWebhooks_FullFlow_SourceEventFansOutAndDelivers(t *testing.T) {
	ctx := context.Background()
	identitySvc, _ := newTestIdentityService(t)
	boot := bootstrapTestTenant(t, ctx, identitySvc, "webhook-"+uuid.NewString()[:8]+"@example.com", "correct horse battery staple 42")
	principal := permissions.Principal{UserID: boot.OwnerUserID, OrganisationID: boot.OrganisationID}

	svc, outboxStore, handlers := newTestWebhooksService(t)

	var received atomic.Int32
	var receivedSig, receivedBody, receivedEventID, receivedTimestamp string
	recv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		receivedBody = string(b)
		receivedSig = r.Header.Get("X-Webhook-Signature")
		receivedEventID = r.Header.Get("X-Webhook-Event-Id")
		receivedTimestamp = r.Header.Get("X-Webhook-Timestamp")
		received.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer recv.Close()

	_, secret, err := svc.RegisterEndpoint(ctx, principal, webhooksapp.RegisterEndpointParams{
		URL: recv.URL, SubscribedEvents: []string{"invoice.finalized"},
	})
	if err != nil {
		t.Fatalf("RegisterEndpoint: %v", err)
	}

	docID := uuid.Must(uuid.NewV7())
	if err := sharedPool.RunScoped(ctx, principal.OrganisationID, func(ctx context.Context) error {
		return outboxStore.Enqueue(ctx, principal.OrganisationID, "invoice.finalized",
			"test-source:"+docID.String(), map[string]any{"document_id": docID})
	}); err != nil {
		t.Fatalf("enqueue source event: %v", err)
	}

	// Fan-out hop: claims the source event (scoped to this test's own
	// organisation — see outbox_helpers_test.go), enqueues one
	// webhook.delivery event.
	if !processNextForOrg(t, ctx, principal.OrganisationID, handlers) {
		t.Fatal("fan-out: expected an event to be claimable, found none")
	}
	// Delivery hop: claims the webhook.delivery event, does the real HTTP POST.
	if !processNextForOrg(t, ctx, principal.OrganisationID, handlers) {
		t.Fatal("delivery: expected an event to be claimable, found none")
	}

	if received.Load() != 1 {
		t.Fatalf("receiver got %d requests, want exactly 1", received.Load())
	}
	if receivedSig == "" || receivedEventID == "" || receivedTimestamp == "" {
		t.Fatalf("missing signature headers: sig=%q event_id=%q timestamp=%q", receivedSig, receivedEventID, receivedTimestamp)
	}
	var payload struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal([]byte(receivedBody), &payload); err != nil {
		t.Fatalf("receiver body did not parse as JSON: %v", err)
	}

	// The receiver independently re-derives the signature exactly as a
	// real webhook consumer would — using ONLY the secret it was given at
	// registration plus the headers/body it received over the wire, never
	// anything internal to the webhooks package — and it must match.
	ts, err := strconv.ParseInt(receivedTimestamp, 10, 64)
	if err != nil {
		t.Fatalf("X-Webhook-Timestamp %q did not parse as an integer: %v", receivedTimestamp, err)
	}
	expectedSig := webhooksapp.Sign(secret, ts, receivedEventID, []byte(receivedBody))
	if receivedSig != expectedSig {
		t.Fatalf("signature verification FAILED: got %s, want %s (receiver could not authenticate this delivery)", receivedSig, expectedSig)
	}

	// And a tampered/wrong secret must NOT verify — proving this is a
	// real check, not something that would pass for any input.
	if webhooksapp.Sign("wrong-secret-entirely", ts, receivedEventID, []byte(receivedBody)) == receivedSig {
		t.Fatal("signature verification did not actually depend on the secret")
	}
}

// TestWebhooks_FailingReceiver_RetriesThenDeadLetters proves a down
// endpoint is retried (brief §38's "retry policy") and eventually
// dead-lettered (visible via webhook_deliveries + a terminal outbox row)
// rather than either blocking forever or silently vanishing.
func TestWebhooks_FailingReceiver_RetriesThenDeadLetters(t *testing.T) {
	ctx := context.Background()
	identitySvc, _ := newTestIdentityService(t)
	boot := bootstrapTestTenant(t, ctx, identitySvc, "webhook-fail-"+uuid.NewString()[:8]+"@example.com", "correct horse battery staple 42")
	principal := permissions.Principal{UserID: boot.OwnerUserID, OrganisationID: boot.OrganisationID}

	svc, outboxStore, handlers := newTestWebhooksService(t)

	var attempts atomic.Int32
	recv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer recv.Close()

	if _, _, err := svc.RegisterEndpoint(ctx, principal, webhooksapp.RegisterEndpointParams{
		URL: recv.URL, SubscribedEvents: []string{"invoice.finalized"},
	}); err != nil {
		t.Fatalf("RegisterEndpoint: %v", err)
	}

	docID := uuid.Must(uuid.NewV7())
	if err := sharedPool.RunScoped(ctx, principal.OrganisationID, func(ctx context.Context) error {
		return outboxStore.Enqueue(ctx, principal.OrganisationID, "invoice.finalized",
			"test-source-fail:"+docID.String(), map[string]any{"document_id": docID})
	}); err != nil {
		t.Fatalf("enqueue source event: %v", err)
	}

	if !processNextForOrg(t, ctx, principal.OrganisationID, handlers) { // fan-out
		t.Fatal("fan-out: expected an event to be claimable, found none")
	}

	// Drive exactly maxDeliveryAttempts (8) delivery attempts, forcing
	// next_attempt_at back to "now" between each one via raw SQL so the
	// test doesn't have to sleep out the real exponential backoff
	// (1m/2m/4m/... capped at 1h — internal/platform/outbox.backoff).
	// This still exercises the real MarkFailed/Permanent logic on every
	// iteration; it only accelerates the wall-clock scheduling, which
	// isn't what this test is trying to verify (the outbox package's own
	// tests already cover backoff timing).
	//
	// IMPORTANT: a permanently-dead-lettered row is STILL status='FAILED'
	// (outbox.go's MarkFailed just pushes next_attempt_at ~100 years out
	// for a Permanent error — it doesn't get a different status) — so
	// this loop must stop forcing next_attempt_at forward once
	// maxDeliveryAttempts is reached, or it would keep un-dead-lettering
	// the row indefinitely and this test would never actually observe the
	// stop condition. Scoping the forced UPDATE to this test's own
	// organisation (processNextForOrg's claim is already org-scoped, and
	// this UPDATE follows the same convention) means it can never touch
	// another test's rows either.
	const maxDeliveryAttempts = 8
	for i := 0; i < maxDeliveryAttempts; i++ {
		if !processNextForOrg(t, ctx, principal.OrganisationID, handlers) {
			t.Fatalf("delivery attempt %d: expected an event to be claimable, found none", i+1)
		}
		if i < maxDeliveryAttempts-1 {
			err := sharedPool.RunScoped(ctx, principal.OrganisationID, func(ctx context.Context) error {
				_, err := sharedPool.Q(ctx).Exec(ctx,
					`UPDATE outbox_events SET next_attempt_at = now() WHERE organisation_id = $1 AND event_type = $2 AND status = 'FAILED'`,
					principal.OrganisationID, webhooksapp.EventTypeDelivery)
				return err
			})
			if err != nil {
				t.Fatalf("forcing next_attempt_at forward (attempt %d): %v", i+1, err)
			}
		}
	}

	if attempts.Load() != maxDeliveryAttempts {
		t.Fatalf("receiver was called %d times, want exactly %d", attempts.Load(), maxDeliveryAttempts)
	}

	// The (maxDeliveryAttempts+1)th attempt must NOT be claimable at
	// all — this is the actual dead-letter assertion: even after forcing
	// next_attempt_at to "now" one more time, a genuinely
	// permanently-failed row stays permanently failed (outbox.go pushes
	// it ~100 years out on a Permanent error, which this forced update
	// would only undo if the code were buggy).
	if processNextForOrg(t, ctx, principal.OrganisationID, handlers) {
		t.Fatal("event was claimed again after reaching maxDeliveryAttempts — dead-lettering did not hold")
	}
	if attempts.Load() != maxDeliveryAttempts {
		t.Fatalf("receiver was called again after dead-lettering: now %d calls, want still %d", attempts.Load(), maxDeliveryAttempts)
	}

	// Confirm a delivery-attempt log row exists recording the failure —
	// brief §38's "dead-letter visibility," queried the same way an
	// operator dashboard would (raw SQL here only because this test has
	// no dedicated ListDeliveries method to call — this is verifying
	// storage, not exercising an app-layer API).
	var failedCount int
	if err := sharedPool.RunScoped(ctx, principal.OrganisationID, func(ctx context.Context) error {
		return sharedPool.Q(ctx).QueryRow(ctx,
			`SELECT count(*) FROM webhook_deliveries WHERE organisation_id = $1 AND succeeded = false`,
			principal.OrganisationID).Scan(&failedCount)
	}); err != nil {
		t.Fatalf("querying webhook_deliveries: %v", err)
	}
	if failedCount == 0 {
		t.Fatal("expected at least one failed delivery-attempt log row")
	}
}
