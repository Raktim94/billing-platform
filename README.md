# billing-platform

[![License: AGPL v3](https://img.shields.io/badge/License-AGPL--3.0-blue.svg)](LICENSE)

A production-grade, multi-company, multi-branch, multi-warehouse billing,
inventory, accounting, and tax-management platform — India GST / e-Invoice /
e-Way Bill capable today, built to extend to other countries' tax regimes
without a rewrite.

This is **not** a CRUD demo. It's built for double-entry accounting
correctness, tenant isolation, and 10-year maintainability from the first
commit — see [`docs/architecture.md`](docs/architecture.md) for the reasoning
behind every major decision.

> **Status:** Stages 0-3 (research, architecture, foundation, catalogue/
> contacts/pricing) are complete and independently verified. Stage 4+ is in
> active, ongoing development. See [`docs/TODO.md`](docs/TODO.md) for
> exactly what's done vs. in progress, and the [wiki](../../wiki) for a
> narrative walkthrough.

## Why this exists

Most "billing software" projects either stay a prototype (float-based money,
editable stock counts, no real double-entry) or copy an existing commercial
product's design wholesale. This one is built from a from-scratch engineering
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

## Core design decisions

| Decision | Why |
|---|---|
| **Modular monolith**, not microservices | Invoice finalization touches inventory, ledger, tax, and numbering atomically — that needs one transaction, not a distributed saga. |
| **`NUMERIC`, never `float`, for money** | Floats can't represent ₹0.01 exactly; a tax/accounting system that rounds wrong is a compliance and trust problem, not a bug ticket. |
| **Append-only stock ledger** | Stock balance is a materialized projection of immutable movements, never a directly-editable number — so it's always reconcilable. |
| **Double-entry, enforced at 3 layers** | App-level sum check, a deferred DB constraint trigger, and no `UPDATE` grant on posted journal rows — a bug in any one layer still can't post an unbalanced or silently-edited journal. |
| **Generic tax model** | `tax_document` / `tax_line` / `tax_component`, not hardcoded `cgst`/`sgst` columns — a `TaxEngine` interface with an `IndiaGSTEngine` plugin, so a second country is additive. |
| **Versioned government adapters** | GSTN changed the e-Invoice/e-Way Bill API schema on 2026-08-01 with weeks' notice — adapters are versioned by directory (`einvoice/v1`, `.../vNext`) so that's a new adapter, not a scramble. |
| **RLS as defense-in-depth** | Every tenant table is scoped by `organisation_id` at the application layer *and* PostgreSQL Row-Level Security — a bug in one layer isn't a cross-tenant data leak. |
| **No bundled reverse proxy** | Self-hosted (CasaOS) and most cloud deployments already have one; TLS termination is the operator's documented job, not a second proxy fighting the first. |

Full reasoning, threat model, ERD shape, and the staged build plan live in
[`docs/architecture.md`](docs/architecture.md).

## Feature scope (target — see TODO for what's actually built)

Quotations, proforma/tax/cash/credit invoices, POS billing, credit/debit
notes, sales & purchase returns · perpetual multi-warehouse inventory with
batch/lot/serial tracking and unit conversion · India GST (CGST/SGST/IGST,
HSN, place-of-supply, inclusive/exclusive pricing) with e-Invoice (IRN/QR)
and e-Way Bill generation · full double-entry accounting with customer/
supplier ledgers, ageing, and GSTR-1/3B-oriented reporting · RBAC with
branch/warehouse-scoped permissions, TOTP MFA, and a full audit trail ·
REST API + an optional read-only-by-default MCP server for AI access ·
self-hosted (Docker/CasaOS) and managed-cloud deployment, with a Tauri 2
desktop shell planned for Windows.

## Tech stack

| Layer | Choice |
|---|---|
| Backend | Go 1.27, `net/http` + [chi](https://github.com/go-chi/chi), [pgx](https://github.com/jackc/pgx) |
| Database | PostgreSQL 18 — `NUMERIC(20,6)`/`NUMERIC(24,12)` for money/rates, Row-Level Security, UUIDv7 primary keys |
| Migrations | [golang-migrate](https://github.com/golang-migrate/migrate), plain versioned SQL |
| Money | [`shopspring/decimal`](https://github.com/shopspring/decimal) wrapped in an internal `Money` type — never `float32`/`float64` |
| Auth | Argon2id (benchmarked params, see [ADR 0001](docs/adr/0001-argon2id-parameters.md)), server-managed sessions, TOTP MFA |
| Observability | `log/slog` (structured JSON) + OpenTelemetry |
| Frontend (planned) | TypeScript, React, Vite, TanStack Query/Router, React Hook Form, Zod, Apache ECharts |
| Desktop (planned) | Tauri 2 — thin shell only, all business logic stays server-side |

## Repository structure

```
apps/           server, worker, web, desktop entrypoints
internal/
  platform/     cross-cutting: config, database, auth, permissions, audit, money, http, observability...
  modules/      domain modules: identity, organisation, catalogue, inventory, sales, accounting, gstindia, einvoice...
api/openapi/    OpenAPI 3.1 spec (source of truth for API clients)
migrations/     versioned SQL migrations
deploy/         docker, compose, casaos deployment manifests
docs/           architecture, research, ADRs, API/operations/security docs
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
git clone https://github.com/Raktim94/billing-platform.git
cd billing-platform

# Point at your own Postgres 18 instance
export DATABASE_DSN="postgres://user:pass@localhost:5432/billing?sslmode=disable"
export DATABASE_AUTO_MIGRATE=true   # applies pending migrations on startup

go build ./...
go run ./apps/server
```

Run the test suite:

```bash
go test ./...                              # unit tests
go test -tags=integration ./...            # integration tests (needs Docker — spins up a real postgres:18 via Testcontainers)
```

See `internal/platform/config/config.go` for the full list of environment
variables (session cookie settings, Argon2id tuning, OTel exporter target,
CORS allow-list, etc.) — every required value fails fast at startup with a
clear message rather than a nil-pointer panic later.

A one-command `./install.sh` + Docker Compose setup (matching
[nodedr-pos](https://github.com/Raktim94/nodedr-pos)'s "clone and run, no
manual config editing" install experience) is planned for Stage 10 —
building against a local Go/Postgres setup, as above, is the current path
while earlier stages are still in progress.

## Documentation

- [`docs/research.md`](docs/research.md) — verified platform/government API
  version facts (not guessed — checked against current sources) and open
  engineering decisions.
- [`docs/architecture.md`](docs/architecture.md) — the full architecture:
  module boundaries, domain model, tax/inventory/accounting design,
  government-integration adapters, security architecture, testing strategy,
  threat model, staged milestone plan.
- [`docs/TODO.md`](docs/TODO.md) — the live build checklist, stage by stage.
- [`docs/adr/`](docs/adr/) — Architecture Decision Records for specific
  non-obvious choices (e.g. Argon2id parameters).
- [Wiki](../../wiki) — narrative documentation: getting oriented in the
  codebase, the tax engine explained, the roadmap in prose form.

## Status and roadmap

Built in explicit, gated stages (research → architecture → foundation →
catalogue → inventory → sales/GST → accounting → reporting → government
integrations → other integrations → packaging → hardening). A stage isn't
marked done until it has real passing unit *and* integration tests — see
[`docs/TODO.md`](docs/TODO.md) for exactly where things stand right now.

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
