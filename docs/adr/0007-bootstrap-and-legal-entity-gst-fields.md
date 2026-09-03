# ADR 0007: expose GSTIN/GSTStateCode on bootstrap and legal-entity endpoints

Status: Accepted
Date: 2026-09-04

## Context

Found by actually running the real HTTP API end to end against a real
database (not by code review, and not by calling the service layer
directly the way every existing test fixture does) — see
`docs/TODO.md`'s note on this session's verification pass. The sequence:

1. `POST /api/v1/auth/bootstrap` — create a fresh organisation.
2. Create a product, a customer, a tax rate.
3. Create and finalize a `TAX_INVOICE`.
4. Finalize failed: `500 INTERNAL_ERROR`, server log showed
   `ERROR: insert or update on table "tax_documents" violates foreign
   key constraint "tax_documents_supplier_state_code_fkey"`.

Root cause: `identity/app.Service.Bootstrap`'s `BootstrapParams` has
carried `GSTIN`/`GSTStateCode` fields since Stage 5b, and the
integration test suite's own fixtures (`tests/integration/sales_test.go`
et al.) call `Bootstrap` directly at the service layer and always pass a
real state code (`"27"`, matching Stage 5a's golden fixtures) — which is
exactly why no test ever caught this. But
`identity/httpapi.bootstrapRequest`, the actual JSON shape a real HTTP
client (including `apps/web`'s own `BootstrapPage.tsx`) sends, never
had these two fields at all. The same gap existed on
`POST /legal-entities`. And there was no `PUT`/`PATCH` for a legal
entity at all, so there was no way to fix an already-bootstrapped
organisation short of direct database access.

**Impact: every real user of this application, through the real UI or
API, could bootstrap an organisation but could never finalize a single
sales document** — `tax_documents.supplier_state_code` is `NOT NULL
REFERENCES gst_state_codes(code)`, and an empty string doesn't satisfy
that constraint (see "Why empty string, not just NULL" below). This is
not a partial-functionality bug; it breaks the core purpose of a billing
platform for literally every real deployment.

## Decision

1. `bootstrapRequest` (identity/httpapi) and `createLegalEntityRequest`
   (organisation/httpapi) now accept `gstin`/`gst_state_code`, threaded
   straight through to the already-existing `BootstrapParams`/
   `CreateLegalEntityParams` fields.
2. Added `PUT /legal-entities/{id}/gst`
   (`organisation.Service.UpdateLegalEntityGST`) — the recovery path for
   every organisation already bootstrapped without a state code before
   this fix, including every org created during this project's own prior
   testing.
3. Added `GET /gst/state-codes` (`gstindia.Service.ListStateCodes`) —
   real reference data so a client can offer a dropdown instead of
   asking a user to type a 2-digit code from memory, which is exactly
   the class of unvalidated/never-collected input that caused this bug
   in the first place.

## Why empty string, not just NULL

`legal_entities.gst_state_code` is nullable — a legal entity in a
country without GST, or genuinely not yet registered, can leave it
unset. The bug wasn't that the column is non-nullable; it's that
`CreateLegalEntityParams.GSTStateCode` defaulted to Go's zero value
(`""`) whenever the HTTP layer never read it from the request, and `""`
is a non-null value that Postgres dutifully checked against
`gst_state_codes(code)` and rejected — except it didn't reject it at
legal-entity-creation time (the insert stores it, and depending on the
exact code path an empty value can round-trip as NULL through
`COALESCE`), it surfaced three steps later, downstream, at invoice
finalization, which is a much harder failure to connect back to its
actual cause. `UpdateGSTDetails` (organisation/pg) uses
`NULLIF($gstin, '')`/`NULLIF($gstStateCode, '')` specifically so a
client explicitly clearing these fields stores a true NULL, not an
empty string masquerading as a value.

## Consequences

- `docs/TODO.md`'s "open questions" note on GST provider choice is
  unaffected — this fix has nothing to do with e-Invoice/e-Way Bill
  provider selection, only with the legal entity's own registered state,
  which every tax calculation (GST or not) needs to determine
  intra-/inter-state treatment.
- `apps/web`'s `BootstrapPage.tsx` and a Settings-page GST-details editor
  still need updating to actually collect/display these fields through
  the UI — the backend fix unblocks the API; the frontend pass is
  tracked as a same-session follow-up in `docs/TODO.md`, not silently
  assumed done.
- Every organisation bootstrapped through this codebase's own prior
  testing (Stage 10a/10b-1/10b-2's manual verification runs) was created
  without a state code and would hit this exact failure if anyone tried
  to finalize an invoice against it — a real, concrete reason to
  prioritize the recovery endpoint (`PUT /legal-entities/{id}/gst`), not
  just the bootstrap fix.
