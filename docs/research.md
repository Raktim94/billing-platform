# Stage 0 — Research

Status: living document. Updated whenever a version or government schema
assumption is verified or changes.

## Confirmed platform versions (verified via web search, 2026-09-02)

| Component | Version | Status | Source |
|---|---|---|---|
| Go | 1.27.1 | Latest stable, released 2026-09-01 | go.dev/doc/devel/release, endoflife.date/go |
| PostgreSQL | 18 (current patch series) | GA since 2025-09-25 | postgresql.org/about/news/postgresql-18-released-3142 |
| GST e-Invoice JSON schema | v1.1 (INV-01) | Current, CBIC/GSTN | einvoice1.gst.gov.in |
| IRP (default) | NIC — einvoice1.gst.gov.in | Sandbox: einv-apisandbox.nic.in | nic.gov.in/project/gst-e-invoice |

**Do not re-trust these numbers indefinitely.** Re-verify Go/Postgres minor
versions before each release build, and re-verify the GST schema version
before every change to `internal/modules/einvoice` or `ewaybill` — GSTN ships
breaking changes with short notice (see below).

## Critical, recently-changed government API behavior

GSTN advisory dated 2026-06-17, deferred from an original 2026-06-15 date and
made **live in production on 2026-08-01**, affecting three APIs simultaneously:
the e-Invoice API, the e-Way Bill by IRN API, and the EWB Closure API.

Two structural changes:

1. **Mandatory Ship-to GSTIN.** Wherever ship-to details are present and an
   e-Way Bill is required, Ship-to GSTIN must now be transmitted. Unregistered
   / not-applicable ship-to party → literal value `URP`.
2. **Voluntary EWB Closure facility.** Supplier, recipient, transporter, or
   even the driver can close an e-Way Bill after delivery (same day or next
   day), via portal login or mobile OTP.

This is a live example of Section 80.13 ("never hard-code government policy
that should be versioned"): had this platform existed with EWB fields
hardcoded, the 2026-08-01 change would have broken production. This is the
concrete justification for `einvoice/v1/`, `einvoice/vNext/` style versioned
adapters from day one (Section 9), and for **not** encoding e-Way Bill closure
as an afterthought — it goes into the state machine now.

Action: the `ewaybill` module's request/response DTOs must include
`ship_to_gstin` (nullable, `URP` sentinel supported) and a `CLOSED` status
distinct from `CANCELLED` from the first migration, even though Stage 8
(actual government wiring) comes later. Getting the schema right now avoids
an awkward migration later.

## Zulivio inspection (Section 27 requirement)

Pending — before implementing `internal/modules/identity`, inspect
`~/Zulivio`'s login UX, change-password UX, password validation, session
handling, rate limiting, and permission model, and record what's reusable
(and what's outdated/not to copy) as an ADR. Zulivio's own memory notes
mention `nest build` diverging from `tsc --noEmit` — irrelevant to Go, but a
reminder to independently verify this new stack's build rather than trust
typecheck-passes-therefore-it-builds.

## Open questions requiring your decision before/around Stage 2

1. **Working name.** Repo scaffolded as `billing-platform` — a placeholder,
   not a product name, chosen specifically to avoid any trademark collision
   while we build. Rename whenever you have a real name; nothing in the
   architecture depends on it.
2. **e-Invoice/e-Way Bill provider.** Do you already hold GSP/ASP credentials
   (or direct NIC IRP API access), or should Stage 8 target the public NIC
   sandbox generically behind `EInvoiceProvider`, with your real provider
   wired in later purely as a new adapter (no domain code changes required,
   by design)?
3. **First deployment target.** CasaOS on debiancasa (consistent with
   KinetiRx/RustFS/Zulivio) or a fresh VPS? Doesn't change the architecture,
   only what Stage 10 gets tested against first.
4. **Country scope for v1.** Assuming India-only for v1 (the generic
   `tax_document/tax_line/tax_component` model supports more later without a
   schema change). Confirm, or tell me if a second country pack is in scope
   for v1.
5. **MFA enforcement timing.** Recommend TOTP mandatory-enforced for
   owners/admins/accountants starting Stage 2 (not deferred) — retrofitting
   auth after other modules depend on the session/permission model is
   materially more expensive than building it in from the start. Confirm.

## Explicitly deferred (not unresolved — decided, just not v1)

- FIFO costing: architecture permits it (costing strategy is pluggable);
  weighted-average only ships in v1.
- MSIX/Microsoft Store signing: Tauri 2 shell prepared from Stage 10; actual
  store submission only after a working desktop build exists to submit.
- NATS: outbox stays Postgres-based until there's a measured scale reason to
  change it.
- Reverse proxy: **not bundled** in the default Compose/CasaOS deployment.
  See `docs/architecture.md` §12 for the reasoning.
