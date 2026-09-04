<p align="center">
  <img src="docs/assets/brand/logo-circle.png" alt="Rechvix logo" width="160">
</p>

<h1 align="center">Rechvix</h1>
<p align="center"><strong>Billing · Inventory · Accounts · GST</strong></p>

<p align="center">
  <a href="LICENSE"><img alt="License: AGPL v3" src="https://img.shields.io/badge/License-AGPL--3.0-blue.svg"></a>
  <img alt="Go version" src="https://img.shields.io/badge/Go-1.27-00ADD8?logo=go&logoColor=white">
  <img alt="Status" src="https://img.shields.io/badge/Status-Stage%2011%20%E2%80%93%20Hardening-orange">
  <a href="https://www.nodedr.com/"><img alt="Made by Nodedr Infotech" src="https://img.shields.io/badge/Made%20by-Nodedr%20Infotech-6b1839"></a>
</p>

A production-grade, multi-company, multi-branch, multi-warehouse billing,
inventory, accounting, and tax-management platform — India GST / e-Invoice /
e-Way Bill capable today, built to extend to other countries' tax regimes
without a rewrite.

This is **not** a CRUD demo. It's built for double-entry accounting
correctness, tenant isolation, and 10-year maintainability from the first
commit — see [`docs/architecture.md`](docs/architecture.md) for the reasoning
behind every major decision.

> **Status:** Stages 0 through 10b-2 — research, architecture, the full
> backend (catalogue, inventory, tax engine, sales, accounting, reporting,
> government integrations, packaging) **and** the web frontend — are
> complete, with real caveats and follow-ups documented inline rather than
> glossed over. Stage 11 (security/performance hardening) is the one
> remaining phase. See [`docs/TODO.md`](docs/TODO.md) for the exact,
> stage-by-stage checklist — including every test count, every known gap,
> and every scope note — and the [wiki](../../wiki) for a narrative
> walkthrough.

## Contents

- [Why this exists](#why-this-exists)
- [Brand](#brand)
- [Core design decisions](#core-design-decisions)
- [What's built](#whats-built)
- [Tech stack](#tech-stack)
- [Repository structure](#repository-structure)
- [Getting started (development)](#getting-started-development)
- [Documentation](#documentation)
- [Status and roadmap](#status-and-roadmap)
- [License](#license)

## Why this exists

Most "billing software" projects either stay a prototype (float-based money,
editable stock counts, no real double-entry) or copy an existing commercial
product's design wholesale. Rechvix is built from a from-scratch engineering
brief that treats the accounting and tax logic as the hard, non-negotiable
part, and the UI as something that has to be fast for a billing-counter
operator, not just pretty.

Same philosophy as this project's sibling,
[nodedr-pos](https://github.com/Raktim94/nodedr-pos): **self-hosted by
default, no external connection required to run your business.** Core
billing, inventory, and accounting work entirely on your own machine —
your own PostgreSQL, no mandatory cloud dependency, no phone-home telemetry.
The only things that ever reach the internet are the integrations you
explicitly turn on and that inherently require it by their own nature — GST
e-Invoice/e-Way Bill submission (a government API, required by Indian law
for those documents), WhatsApp/email sharing, and optional cloud object
storage. Everything else — creating invoices, tracking stock, running
reports, closing the books — works fully offline.

## Brand

<p align="center">
  <img src="docs/assets/brand/banner-hero.png" alt="Rechvix brand banner — From Products to Profits, Simplified" width="100%">
</p>

<p align="center">
  <img src="docs/assets/brand/banner-secondary.png" alt="Rechvix brand banner — A complete business management platform for a simpler, smarter tomorrow" width="100%">
</p>

The two banners above are brand/marketing artwork, not application
screenshots — Rechvix's real frontend (a working Vite/React app covering
Dashboard, Sales, Purchases, Inventory, Contacts, Accounting, GST/Tax, and
Reports — see [What's built](#whats-built)) already exists and is
functional, but real UI screenshots aren't included in this pass yet. A
square logo variant is also available at
[`docs/assets/brand/logo-square.png`](docs/assets/brand/logo-square.png).

## Core design decisions

| Decision | Why |
|---|---|
| **Modular monolith**, not microservices | Invoice finalization touches inventory, ledger, tax, and numbering atomically — that needs one transaction, not a distributed saga. |
| **`NUMERIC`, never `float`, for money** | Floats can't represent ₹0.01 exactly; a tax/accounting system that rounds wrong is a compliance and trust problem, not a bug ticket. |
| **Append-only stock ledger** | Stock balance is a materialized projection of immutable movements, never a directly-editable number — so it's always reconcilable. |
| **Double-entry, enforced at 3 layers** | App-level sum check, a deferred DB constraint trigger, and an unconditional trigger making posted journal rows immutable (a trigger, not just `REVOKE UPDATE` — a table owner bypasses grants the same way it bypasses RLS) — a bug in any one layer still can't post an unbalanced or silently-edited journal. Both enforcement mechanisms are proved with raw SQL that bypasses the Go app layer entirely, not just unit-tested. |
| **Generic tax model** | `tax_document` / `tax_line` / `tax_component`, not hardcoded `cgst`/`sgst` columns — a `TaxEngine` interface with an `IndiaGSTEngine` plugin, so a second country is additive. |
| **Versioned government adapters** | GSTN changed the e-Invoice/e-Way Bill API schema on 2026-08-01 with weeks' notice — adapters are versioned by directory (`einvoice/v1`, `.../vNext`) so that's a new adapter, not a scramble. |
| **RLS as defense-in-depth** | Every tenant table is scoped by `organisation_id` at the application layer *and* PostgreSQL Row-Level Security — a bug in one layer isn't a cross-tenant data leak. |
| **No bundled reverse proxy** | Self-hosted (CasaOS) and most cloud deployments already have one; TLS termination is the operator's documented job, not a second proxy fighting the first. |

Full reasoning, threat model, ERD shape, and the staged build plan live in
[`docs/architecture.md`](docs/architecture.md).

## What's built

Nearly everything in the original brief is implemented, tested, and
independently re-verified — [`docs/TODO.md`](docs/TODO.md) is the exact,
stage-by-stage source of truth (including every test count and every
documented gap); this is the summary:

- **Catalogue, contacts, pricing** — products/variants/SKUs/barcodes,
  units of measure with auditable conversions, customers/suppliers with
  GST fields, multi-currency price lists.
- **Inventory** — perpetual multi-warehouse stock via an append-only
  movement ledger, batch/lot/serial tracking, weighted-average costing,
  reservations and reorder policies, concurrency-tested against real
  oversell and lost-update races.
- **Tax engine** — a generic `TaxEngine` interface with a full
  `IndiaGSTEngine`: CGST/SGST/UTGST vs. IGST, cess, HSN, place-of-supply,
  inclusive/exclusive pricing, golden fixtures checked by hand against
  brief figures.
- **Sales & purchase documents** — quotations, proforma, sales orders,
  delivery challans, tax/POS invoices, credit/debit notes, sales returns,
  recurring invoices, purchase orders/GRN/purchase invoices/returns —
  finalization is one atomic transaction touching tax, inventory, and
  numbering together, with PDF output (A4, thermal 80mm/58mm, and more).
- **Double-entry accounting** — chart of accounts, journals, fiscal
  periods, receipts/payments/reconciliations, auto-posting from finalized
  documents, customer/supplier ledgers derived fresh from journal lines
  (never a mutable balance column), ageing, fiscal-year locking.
- **Reports & dashboard** — sales/purchase/inventory/accounting/tax
  reports, GSTR-1-oriented preparation (explicitly labeled as prep, not a
  filing submission), CSV/XLSX/JSON/PDF export, a live 8-card dashboard.
- **Government integrations** — a real transactional outbox +
  background worker, e-Invoice (IRN/QR) against the NIC sandbox, and a
  free-first e-Way Bill workflow (generate/retrieve/cancel/Part-B history,
  plus a "prepare → open the government portal → enter the result"
  no-paid-API path) with a persisted status machine.
- **Integrations** — scoped, revocable API keys as a real session
  alternative, HMAC-signed webhooks with retry/backoff and a delivery log,
  and a real MCP server (official `modelcontextprotocol/go-sdk`) exposing
  read-only, permission-checked tools for AI access.
- **RBAC & security** — TOTP MFA with recovery codes, Argon2id password
  hashing, a full audit trail, and Row-Level Security on every tenant
  table.
- **Packaging** — multi-stage Docker images, Docker Compose (Postgres +
  migration job + app + worker, no bundled reverse proxy), and a CasaOS
  manifest — actually run end-to-end (`docker compose up` →
  bootstrap → healthy), not just written.
- **Frontend** — a real, working web app (not a mockup): Dashboard,
  Sales (barcode/keyboard-driven billing counter), Purchases, Inventory,
  Contacts, Catalogue, Accounting, GST/Tax, Reports, Integrations, and
  Settings screens, all wired to the tested backend APIs above, with
  light/dark themes, global search, and a WCAG 2.2 AA accessibility pass.

**Known, deliberately documented gaps** (not silently skipped — see
`docs/TODO.md` for the full, stage-by-stage list): purchases post a single
line-total to the ledger rather than a split GST input-tax-credit line, so
GSTR-3B-oriented reporting isn't built yet; email/SMS/WhatsApp providers
are interfaces only (no free public sandbox exists for any of them, unlike
the government tax APIs); Redis/MinIO are provisioned in Compose but
genuinely unwired; the Tauri 2 desktop shell hasn't been started; and
Stage 11's full security/performance hardening pass (load testing at
100k+ products, a complete IDOR/CSRF/session-fixation review, backup/
restore verification) is still in progress.

## Tech stack

| Layer | Choice |
|---|---|
| Backend | Go 1.27, `net/http` + [chi](https://github.com/go-chi/chi), [pgx](https://github.com/jackc/pgx) |
| Database | PostgreSQL 18 — `NUMERIC(20,6)`/`NUMERIC(24,12)` for money/rates, Row-Level Security, UUIDv7 primary keys |
| Migrations | [golang-migrate](https://github.com/golang-migrate/migrate), plain versioned SQL |
| Money | [`shopspring/decimal`](https://github.com/shopspring/decimal) wrapped in an internal `Money` type — never `float32`/`float64` |
| Auth | Argon2id (benchmarked params, see [ADR 0001](docs/adr/0001-argon2id-parameters.md)), server-managed sessions, TOTP MFA |
| Observability | `log/slog` (structured JSON) + OpenTelemetry |
| Frontend | Vite + React 19 + TypeScript (strict mode), TanStack Query/Router, React Hook Form + Zod, Apache ECharts, self-hosted fonts (no external font/CDN calls) |
| Desktop | Tauri 2 planned — a thin shell around the same web build, no business logic duplicated; not started yet |

## Repository structure

```
apps/           server, worker, mcp, web entrypoints (desktop planned)
internal/
  platform/     cross-cutting: config, database, auth, permissions, audit, money, http, observability...
  modules/      domain modules: identity, organisation, catalogue, inventory, sales, accounting, gstindia, einvoice...
api/openapi/    OpenAPI 3.1 spec (source of truth for API clients) — all 124 real routes, mechanically verified
migrations/     versioned SQL migrations
deploy/         docker, compose, casaos deployment manifests
docs/           architecture, research, ADRs, API/operations/security docs, brand assets
tests/          integration and end-to-end tests
```

Each `internal/modules/*` package follows `domain/` (pure business rules,
no I/O) → `app/` (use-case orchestration) → `pg/` (repository
implementation) layering — see `internal/modules/organisation` for the
reference shape.

## Getting started (development)

Requires Go 1.27+ (or let `go`'s toolchain auto-download it — the module
already pins the version) and a PostgreSQL 18 instance.

```bash
git clone https://github.com/Raktim94/rechvix.git
cd rechvix

# Point at your own Postgres 18 instance
export DATABASE_DSN="postgres://user:pass@localhost:5432/billing?sslmode=disable"
export DATABASE_AUTO_MIGRATE=true   # applies pending migrations on startup

go build ./...
go run ./apps/server
```

Run the frontend against it:

```bash
cd apps/web
npm install
npm run dev
```

Run the test suite:

```bash
go test ./...                              # unit tests
go test -tags=integration ./...            # integration tests (needs Docker — spins up a real postgres:18 via Testcontainers)
```

Or bring up the whole backend stack (Postgres + migrations + server +
worker) with Docker Compose:

```bash
cd deploy/compose
docker compose up -d
```

See `internal/platform/config/config.go` for the full list of environment
variables (session cookie settings, Argon2id tuning, OTel exporter target,
CORS allow-list, etc.) — every required value fails fast at startup with a
clear message rather than a nil-pointer panic later. A one-command
`./install.sh` matching [nodedr-pos](https://github.com/Raktim94/nodedr-pos)'s
"clone and run" experience is Stage 11 scope; the steps above are the
current path.

## Documentation

- [`docs/research.md`](docs/research.md) — verified platform/government API
  version facts (not guessed — checked against current sources) and open
  engineering decisions.
- [`docs/architecture.md`](docs/architecture.md) — the full architecture:
  module boundaries, domain model, tax/inventory/accounting design,
  government-integration adapters, security architecture, testing strategy,
  threat model, staged milestone plan.
- [`docs/api.md`](docs/api.md) / [`api/openapi/openapi.yaml`](api/openapi/openapi.yaml) — the REST API reference and machine-readable spec, verified against every real route registration.
- [`docs/database.md`](docs/database.md) — the schema, cross-checked against every migration.
- [`docs/operations/deployment.md`](docs/operations/deployment.md) — real deployment commands and reasoning, gaps stated plainly.
- [`docs/TODO.md`](docs/TODO.md) — the live, stage-by-stage build checklist — the single most accurate source for "what's actually done."
- [`docs/adr/`](docs/adr/) — Architecture Decision Records for specific non-obvious choices (e.g. Argon2id parameters, `GetByID` organisation scoping).
- [`CHANGELOG.md`](CHANGELOG.md) — derived from real git history, one entry per shipped stage.
- [Wiki](../../wiki) — narrative documentation: getting oriented in the codebase, the tax engine explained, the roadmap in prose form.

## Status and roadmap

Built in explicit, gated stages (research → architecture → foundation →
catalogue → inventory → tax engine → sales → accounting → reporting →
government integrations → other integrations → packaging → frontend →
hardening). A stage isn't marked done until it has real passing unit *and*
integration tests, independently re-verified — see
[`docs/TODO.md`](docs/TODO.md) for the exact test counts and the specific,
named gaps at every stage. As of this writing: **Stages 0 through 10b-2 are
complete** (full backend, government integrations, Docker/CasaOS packaging,
and a working frontend with a WCAG 2.2 AA accessibility pass). **Stage 11
— security and performance hardening — is the one phase still in
progress**: a full IDOR/CSRF/session-fixation/webhook-replay review, load
testing at realistic scale, backup/restore verification, and a migration
test against a prior schema.

## License

Copyright © 2026 [Nodedr Infotech Private Limited](https://www.nodedr.com/)
and Raktim Ranjit. Licensed under the
[GNU Affero General Public License v3.0](LICENSE) (AGPL-3.0). In short:
you're free to self-host, use, and modify this software for your business.
If you modify it and run that modified version as a network service for
others, you must make your modified source available to those users under
the same license — this keeps improvements to a self-hosted business tool
in the open rather than disappearing into a closed commercial fork. Same
license as this project's sibling, [nodedr-pos](https://github.com/Raktim94/nodedr-pos).
