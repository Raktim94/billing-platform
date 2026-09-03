# ADR 0005: Webhook delivery via two-hop outbox fan-out

## Status
Accepted (Stage 9).

## Context
Brief §38 requires webhook delivery to be retried independently per
endpoint, with exponential backoff and eventual dead-letter visibility.
Stage 8 already built a generic transactional outbox
(`internal/platform/outbox`) specifically so Stage 9 could reuse it rather
than build a second queue.

A single outbox row can only carry one status/attempts/next-retry state.
If a business event (e.g. `sales.FinalizeDocument` finalizing an invoice)
enqueued exactly one outbox event per event *occurrence*, and a single
handler then looped over every subscribed endpoint inside that one
handler invocation, one endpoint being down would either (a) block/delay
delivery to healthy endpoints until the down one's retry backoff clears,
or (b) require the handler to swallow per-endpoint errors and invent its
own secondary retry bookkeeping — reimplementing what the outbox already
does.

## Decision
Two hops through the same outbox table:

1. A producer module (`sales`, `einvoice`) enqueues ONE outbox event
   describing the domain occurrence (`event_type` = the brief §38 catalog
   name, e.g. `invoice.finalized`), exactly the same call shape as Stage
   8's `einvoice.generate` enqueue — no webhooks-specific code in the
   producer.
2. `webhooks.Service.HandlerForSourceEvent(eventType)` is registered
   against each of those event types in `apps/worker`. On claim, it looks
   up every active endpoint subscribed to that event type and enqueues
   ONE `webhook.delivery` outbox event per endpoint, with an idempotency
   key of `sourceEventID + endpointID` — so re-processing the source event
   (a worker restart mid-fan-out) can never double-enqueue a delivery for
   the same endpoint.
3. `webhooks.Service.DeliverHandler()`, registered against
   `webhook.delivery`, performs the actual signed HTTP POST for one
   (event, endpoint) pair, with its own independent retry/backoff via the
   outbox's existing `MarkFailed` mechanism.

## Consequences
- Each endpoint's health is fully independent — a down endpoint's retries
  never affect any other endpoint's delivery timing.
- No second retry/backoff implementation: `webhooks` calls
  `outbox.Permanent` after `maxDeliveryAttempts` to dead-letter a
  delivery, exactly like `einvoice` does for a genuinely non-retryable
  request.
- Cost: one extra outbox round-trip per delivery (fan-out row, then
  delivery row) versus a hypothetical single-hop design. Judged
  acceptable — this system's delivery volume (per-organisation business
  events, not high-frequency telemetry) does not make that overhead
  material, and the alternative (a bespoke second queue, or per-endpoint
  retry logic bolted onto the fan-out handler) is more code to maintain
  for a correctness property the outbox already provides for free.
- `webhook_deliveries` (migrations/0026) is a human-readable attempt log
  for operator visibility, distinct from the outbox row's own
  status/attempts, which remains the authoritative retry state.
