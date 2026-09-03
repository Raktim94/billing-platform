# API

This is a narrative overview of conventions that hold across the whole
REST API. It is not the exhaustive per-endpoint reference — that's
`api/openapi/openapi.yaml` (source of truth for exact request/response
shapes; in progress, see that file's own status note at the top). This
document exists so a new client integrator understands the *shape* of the
API before reading the spec line by line.

## Base path and versioning

Every route is mounted under `/api/v1` (`apps/server/main.go`). There is
no per-endpoint version — a breaking change gets a new top-level prefix
(`/api/v2`) when that day comes, not per-route version headers.

## Authentication

Two methods, and any authenticated route accepts either
(`identity/httpapi.RequireAuthOrAPIKey`):

1. **Session cookie** — `bp_session` by default (`SESSION_COOKIE_NAME`),
   HttpOnly, set by `POST /api/v1/auth/login`. This is what `apps/web`
   uses; the frontend never holds a token, only a non-secret
   `{orgId, userId}` UI hint (`apps/web/src/auth/session.ts`).
2. **API key** — `Authorization: Bearer <key>`, created via the API-key
   management endpoints (identity module). Keys are high-entropy, shown
   once, hashed at rest, individually revocable, with optional expiry and
   IP restriction, and carry a fixed-vocabulary scope list:
   `products:read`, `inventory:read`, `customers:read`,
   `customers:write`, `invoices:read`, `invoices:write`, `reports:read`
   (`internal/modules/identity/domain.APIScope`). A key's effective
   permissions are the **intersection** of its declared scopes and its
   owning user's real RBAC grants — never a wildcard, never scope alone.

Routes that must stay reachable without a session (login itself,
first-run bootstrap, password-reset request/completion) are mounted
outside `RequireAuth`/`RequireAuthOrAPIKey` — see the route tree in
`apps/server/main.go`.

## Errors

Every error response is the same JSON envelope
(`internal/platform/http/errors.go`), regardless of which module raised
it:

```json
{
  "error": {
    "code": "SESSION_INVALID",
    "message": "Your session has expired. Please sign in again.",
    "details": { "...": "optional, machine-readable, endpoint-specific" },
    "request_id": "..."
  }
}
```

An error the application layer didn't explicitly construct (a panic, an
unexpected DB error, ...) is always reduced to a generic `500
INTERNAL_ERROR` with no internal detail beyond the `request_id` — the
full error is logged server-side, never sent to the client. Quote the
`request_id` when asking for support on a `500`.

## Money

Every monetary field on the wire is an object, not a bare number:

```json
{ "amount": "1234.50", "currency": "INR" }
```

`amount` is a **string**, deliberately — see `internal/platform/money`'s
own doc comment and `apps/web/src/lib/money.ts`'s mirror of it. A JS
`number`/float cannot losslessly represent an arbitrary-precision
decimal; every client is expected to treat `amount` as an opaque string
for anything beyond display formatting (`Intl.NumberFormat` or
equivalent), and to never recompute a total client-side for anything that
mutates state — the server is the sole source of calculation truth.

## Lists and filtering

List endpoints take `q` for free-text search (e.g.
`GET /contacts/parties?q=...`) and return the full matching set — there
is no cursor/offset pagination convention in this API yet. This is a real
scale gap, not a design choice with headroom already built in; see Stage
11's load-testing item in `docs/TODO.md` for when this needs revisiting
(the brief's target scale is 100k products / 1M invoice lines).

## Report endpoints

Every `/reports/*` endpoint shares one shape
(`internal/platform/export.Table`: `{title, headers, rows}`, `rows`
pre-stringified) and accepts `?format=` — `json` (default), `csv`,
`xlsx`, or `pdf` — via one shared writer (`internal/platform/export`).
This is why `apps/web`'s `ReportTable` component can be one generic
renderer pointed at any report path rather than one hand-built component
per report.

## MCP (read-only AI access)

`apps/mcp` exposes the same application-layer methods a REST client would
call, over the Model Context Protocol instead of HTTP — 10 read-only
tools (brief §39), scoped by the same API-key mechanism above (an MCP
client authenticates with an API key, resolved once at process startup;
no tool's input schema carries an `organisation_id` field at all, so
cross-tenant access isn't just checked, it's structurally impossible to
request). Write tools (`create_invoice_draft` etc.) are explicitly out of
scope for now — see `docs/TODO.md` Stage 9.

## Webhooks

Outbound only, for now: `invoice.finalized`, `einvoice.generated`, and
`einvoice.failed` are the only wired source events (Stage 9's brief §38
catalog is broader — `invoice.created`/`cancelled`, `payment.*`,
`stock.*`, `customer.created`, `ewaybill.*` remain unwired producer-side,
tracked in `docs/TODO.md`). Every delivery is HMAC-SHA256 signed over
`timestamp + eventID + body`; verify the signature before trusting a
payload. Deliveries retry with backoff via the outbox mechanism and
dead-letter after 8 attempts — `webhook_deliveries` is the visibility log
for what was actually sent and when.

## Module → base path map

| Module | Base path(s) (verified against each module's `Mount()`, not guessed) |
|---|---|
| Identity | `/auth/*`, `/api-keys/*` |
| Organisation | `/organisation`, `/legal-entities`, `/branches`, `/warehouses` |
| Catalogue | `/catalogue/*` |
| Contacts | `/contacts/*` |
| Pricing | `/pricing/*` |
| Inventory | `/inventory/*` |
| Purchases | `/purchases/*` |
| GST / tax | `/gst/*` |
| Sales | `/sales/*` |
| Accounting | `/accounting/*` |
| Reporting | `/reports/*` |
| e-Way Bill | `/ewaybill/*`, plus `/sales/documents/{id}/ewaybill*` |
| Logistics | `/logistics/*` |
| Notifications | `/notifications/send`, `/share-links`, `/share/*` (redemption, unauthenticated) |
| Webhooks | `/webhooks/*` |
