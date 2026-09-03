# Build TODO

Master task list for the whole project, staged per `docs/architecture.md`
§16. Checked items are actually built, tested, and merged — not just
started. This file is the source of truth for "what's done"; update it as
part of finishing a stage, not after the fact.

## Stage 0 — Research ✅ (2026-09-02)
- [x] Verify Go/PostgreSQL current versions
- [x] Verify current GST e-Invoice/e-Way Bill API schema state
- [x] `docs/research.md` produced

## Stage 1 — Architecture ✅ (2026-09-02)
- [x] Module diagram, ERD shape, layering rules
- [x] Tax/inventory/accounting/GST/e-Invoice/e-Way Bill architecture
- [x] Threat model, security architecture
- [x] Docker/CasaOS/Tauri plan (no bundled reverse proxy, reasoning documented)
- [x] `docs/architecture.md` produced, user confirmed ("ok do it")

## Stage 2 — Foundation ✅ (2026-09-02)
- [x] `internal/platform/{config,logging,database,observability,http,crypto,permissions,audit,money}`
- [x] Migrations: organisations → warehouses, RBAC catalog, users, sessions, MFA, password reset, audit_log — RLS on every tenant table
- [x] `internal/modules/identity` — login, sessions, TOTP MFA + recovery codes, password change/reset, bootstrap
- [x] `internal/modules/organisation` — org/legal-entity/branch/warehouse CRUD
- [x] `apps/server` composition root
- [x] ADR: Argon2id parameters (benchmarked)
- [x] Unit tests (46/46) + Testcontainers integration tests incl. RLS cross-tenant isolation (6/6), independently re-verified
- [x] `docs/adr/0001-argon2id-parameters.md`

## Stage 3 — Catalogue / Contacts / Pricing ✅ mostly (2026-09-02, one carry-over)
- [x] `internal/modules/catalogue`: products, product_variants, SKUs, barcodes, categories, brands
- [x] Units of measure + explicit unit_conversions (BOX→PCS, etc.), auditable
- [x] `internal/modules/contacts`: parties (customer/supplier/both), addresses (billing/shipping/warehouse/registered), tax registrations, credit limit, payment terms
- [x] `internal/modules/pricing`: price lists, price list items, multi-currency price fields (uses `internal/platform/money`)
- [x] GST fields on catalogue (HSN/SAC) and contacts (GSTIN, state code) — data only, no tax calculation logic yet (that's Stage 5/`gstindia`)
- [x] Global search groundwork: trigram indexes on product name/SKU (contacts trigram search: verify alongside the carry-over item below)
- [ ] **Carried to Stage 4:** import/export scaffolding for products/customers/suppliers (CSV/XLSX, dry-run + validation report, brief §53) — not built in the Stage 3 pass, genuinely missing, not deferred by decision
- [x] Unit tests (63/63) + integration tests (15/15 incl. per-module RLS cross-org checks), independently re-verified

## Stage 4 — Inventory ✅ (2026-09-03, one scope note)
- [x] `internal/modules/inventory`: stock_movements (append-only), stock_balances (materialized, app-layer-maintained — see `docs/adr/0002-stock-balance-maintenance.md`)
- [x] Movement types per brief §11 (OPENING, PURCHASE_RECEIPT, PURCHASE_RETURN, SALE, SALE_RETURN, TRANSFER_IN/OUT, ADJUSTMENT_IN/OUT, ASSEMBLY_IN/OUT, DAMAGE, EXPIRY) — SALE/SALE_RETURN/ASSEMBLY_* wired as movement-recording primitives only; the sales/assembly documents that call them are Stage 5+ scope
- [x] Batches (batch+lot modeled as one concept, see migration comment), serial numbers, expiry/manufacturing dates
- [x] Stock reservations, low stock/reorder/safety stock (`stock_policies`)
- [x] Weighted-average costing strategy (unit-conversion-normalized — real bug caught and fixed in dev, see `docs/adr/0002-stock-balance-maintenance.md` and `TestInventory_UnitConversionAwareReceipt`)
- [x] `internal/modules/purchases`: purchase order, goods receipt (GRN — posts stock), purchase invoice, purchase return (reverses stock), debit note. **Supplier payment/credit deliberately NOT built** — needs Stage 6's ledger/payment infrastructure to post against; a payment table with nothing to apply it to would just be a stub. Permissions reuse Stage 2's pre-seeded `purchase.*` codes (not a new `purchases.*` namespace)
- [x] Concurrency tests: last-unit oversell race (only one of 8 concurrent takers wins), concurrent multi-warehouse transfer (no lost/duplicated stock), concurrent document-number allocation (Scenario D/I), all re-run multiple times for stability
- [x] Unit tests (78/78) + integration tests (39/39, incl. RLS sweep across every new table) — independently re-verified
- [x] **Carried-over CSV/XLSX import from Stage 3, completed this stage**: `internal/platform/importer` (shared parse + dry-run/report scaffolding) wired into `catalogue.ImportProducts` and `contacts.ImportParties`, with dedup + per-row validation reporting

## Stage 5a — Tax Engine ✅ (2026-09-03)
- [x] `internal/modules/taxation`: generic `TaxEngine` interface, `Money`-based calculation, tax_document/tax_line/tax_component model, tax_rate_master with valid_from/valid_to
- [x] `internal/modules/gstindia`: `IndiaGSTEngine` — GSTIN, place of supply, intra/inter-state CGST+SGST/UTGST vs IGST, cess, HSN, exempt/nil-rated/zero-rated, B2B/B2C/export/SEZ carried on tax_document; reverse-charge as a caller-set flag (liability accounting is Stage 6)
- [x] Golden tax fixtures: 0/3/5/12/18/28/40%, ₹90 inclusive @18% fixture (verified against the brief's exact 76.271186.../13.728813... figures), intra-state CGST+SGST split, inter-state IGST, cess-on-taxable-not-gross, UTGST, valid_from/valid_to snapshot immutability — independently re-verified, numbers checked by hand against the actual test code, not just trusted
- [x] Unit tests (96/96) + integration tests (47/47) — independently re-verified, 2 real bugs caught by tests and fixed (RLS-scope-before-calculation, CHECK-constraint default)

## Stage 5b — Sales Documents & Printing ✅ (2026-09-03)
- [x] `internal/modules/sales`: quotation, proforma, sales order, delivery challan, tax invoice, POS invoice, credit/debit note, sales return, recurring invoice (document_type-parameterized, one family); `ConvertDocument` generically covers quotation/sales-order/challan → invoice
- [x] Invoice header + line fields per brief §5, calculation snapshot immutability (brief §55, tested against a live tax-rate-master change) — calls Stage 5a's `TaxEngine` via new `taxation.CalculateAndSnapshotTx`, calls `inventory.RecordMovementForOtherModule` on finalize, all inside one transaction
- [x] Document numbering: `internal/platform/numbering`, configurable series scoped to organisation/branch/document-type/financial-year (brief §51), concurrency-safe allocation verified under real concurrent load (Scenario I) — the real system purchases' Stage 4 counter was a placeholder for; purchases itself not yet migrated onto it (follow-up)
- [x] Printing/PDF template engine (`internal/modules/sales/printing`, go-pdf/fpdf): A4 GST invoice, compact, thermal 80mm/58mm, quotation, PO, challan, receipt, statement, credit/debit note (brief §19) — "Previous Balance" field intentionally stubbed nil until Stage 6's ledger exists
- [x] Sales screen API groundwork: `BillingLookup` (product search + stock + price in one call, brief §24/§25)
- [x] Unit tests (100/100) + integration tests (53/53, incl. finalize atomicity, numbering concurrency, tax-snapshot immutability, RLS, PDF rendering) — independently re-verified, one real bug caught by the print test and fixed (RLS-scope-before-read ordering, same class as earlier stages)

## Stage 6 — Accounting ✅ (2026-09-03, one scope note)
- [x] `internal/modules/accounting`: chart_of_accounts (idempotent default-seed via `EnsureDefaultChartOfAccounts`), journals, journal_lines, fiscal_periods, receipts, payments, bank_accounts, reconciliations
- [x] Double-entry invariant enforced at 3 layers per `docs/architecture.md` §7 — app-layer `validateBalanced` (unit + property tested), a Postgres DEFERRED CONSTRAINT TRIGGER re-verifying Σdebit=Σcredit at COMMIT (proved via raw SQL bypassing the Go app layer entirely), and an unconditional BEFORE UPDATE/DELETE trigger making journal_lines immutable from the instant they're written (also proved via raw SQL) — a trigger rather than GRANT/REVOKE specifically because a table owner bypasses grants the same way it bypasses RLS (Stage 2's lesson), so a trigger is the one mechanism guaranteed to hold regardless of which role connects
- [x] Auto-posting from finalized sales/purchase documents (`docs/adr/0003-accounting-integration-point.md`) — sales posts Dr AR/Cr Sales+Tax Payable (or reversed for credit notes/returns) using the real tax snapshot; purchases posts Dr Purchases/Cr AP for invoice/return/debit-note. **Scope note, not silently skipped:** purchases posts a single line-total amount, not split into a separate GST Input Tax Credit line — purchases has never been wired through the tax engine (unlike sales), and doing that properly is flagged as a bounded, well-precedented follow-up in the ADR rather than rushed here
- [x] Customer/supplier ledger (`GetPartyLedger`) — derived fresh from journal_lines every call, never a mutable stored balance column (explicitly avoiding nodedr-pos's real Float-`increment` balance-drift incident); ageing (`GetAgeing`) via oldest-first FIFO credit-to-debit matching
- [x] Fiscal year config + period locking (brief §52) — `accounting.override_locked_period` is a separate permission from `accounting.post`; lock lifecycle tested, override-path tested (the bootstrap Owner holds every permission so the "non-override user is blocked" direction isn't separately covered — flagged, not silently assumed)
- [x] Property test: Σdebit == Σcredit for any valid journal (200 randomized generate-then-check iterations, plus a perturbation check that a 1-paisa-unbalanced journal is always rejected)
- [x] Scenario E passing end-to-end: ₹10,000 tax-inclusive credit sale → ₹4,000 receipt → ledger shows exactly ₹6,000 outstanding
- [x] Unit tests (105/105) + integration tests (62/62, incl. RLS on journals/journal_lines via raw SQL with no org filter in the query — the RLS-itself test, not an app-layer WHERE-clause test) — independently re-verified, one real bug caught by the test suite and fixed (migration declared `NOT NULL DEFAULT ''` on `description`/`reference_number` columns, inconsistent with the nullable+`nullIfEmpty`/`COALESCE` convention every other module in this codebase uses — fixed in the migration before it was ever pushed)

## Stage 7 — Reports / Dashboard ✅ (2026-09-03, two scope notes)
- [x] Sales: summary (day/month/customer/product/category/salesperson/branch/warehouse), invoice detail, gross profit (**approximate COGS** — current `average_cost`, not historical-at-sale-time; see `domain.GrossProfitRow`'s doc comment and follow-up note below)
- [x] Purchases: summary (by supplier/product/etc.), document detail — no taxable/tax breakdown yet (purchases still isn't wired through the tax engine, a known gap flagged since Stage 4/6)
- [x] Inventory: stock valuation, low stock, stock movements
- [x] Accounting: trial balance, org-wide receivables/payables (batched over Stage 6's existing per-party ageing), account ledger (cash/bank book)
- [x] Tax: HSN summary, tax-rate summary, GSTR-1-oriented preparation (explicitly labeled NOT a filing submission in both the API response and UI-facing title string) — **GSTR-3B-oriented summary deliberately NOT built**: its inward/ITC side needs purchases' tax-engine wiring, which doesn't exist; building only the outward half risked presenting an incomplete GSTR-3B as if complete, so it's deferred as a real gap rather than shipped half-true
- [x] Export: CSV/XLSX/JSON/PDF via one shared `internal/platform/export` writer, `?format=` query param on every report endpoint — synchronous only; the background-job + expiring-link half of brief §54 needs `apps/worker`/an outbox mechanism that doesn't exist until Stage 9, documented as a follow-up rather than faked
- [x] Dashboard: all 8 cards, live indexed queries per `docs/adr/0004-dashboard-query-design.md` (not a materialized table — reasoning + a measured sanity check at ~500+200 documents documented there)
- [x] Shared filter-building helper (`internal/modules/reporting/pg`'s `whereBuilder`) + a validated `GroupDimension` allow-list for GROUP BY (never raw-string SQL, brief §62)
- [x] Unit tests (10 new) + integration tests (11 new, incl. a report-specific cross-organisation RLS-leak test across 5 report types and a dashboard performance sanity check) — independently re-verified, one real bug caught and fixed (ambiguous `organisation_id` column reference on every multi-table-join report query — ~10 query sites)

## Stage 8 — Government Integrations (sandbox only) ✅ (2026-09-03, follow-ups noted)
- [x] `internal/platform/outbox`: generic transactional outbox (`outbox_events`, a SECURITY DEFINER `outbox_claim_next()` for the one deliberate cross-org RLS bypass a background poller needs — same pattern as migrations/0003's login lookup), `Poller` with exponential backoff — this did not exist before Stage 8 and is built reusable for Stage 9, not e-Invoice-specific
- [x] `apps/worker`: real, separate composition-root process running the outbox poller — never inline with the HTTP request path
- [x] `internal/modules/einvoice`: `EInvoiceProvider` interface (exact signature from `docs/architecture.md` §9), `v1/mock` (canned, deterministic, zero network calls — the only provider automated tests ever exercise) and `v1/sandbox` (real NIC-sandbox-calling adapter, wired into `apps/worker` behind `EINVOICE_PROVIDER=sandbox`, reviewed-by-reading not proven-by-testing — brief Rule 17), persists IRN/ack/signed QR/status/correlation id per §9's field list
- [x] `internal/modules/ewaybill`: generate/retrieve/cancel/Part-B history/voluntary closure — **Ship-to GSTIN (URP sentinel) + CLOSED status distinct from CANCELLED** (2026-08-01 GSTN change, `docs/research.md`) present from the first migration, round-trip tested
- [x] Explicit persisted status machine (DRAFT→QUEUED→SUBMITTING→GENERATED|FAILED_RETRYABLE|FAILED_FINAL, CANCEL_PENDING→CANCELLED, CLOSED) — real CHECK-constrained columns, not inferred
- [x] Never called from sales domain code directly — `sales.FinalizeDocument` only enqueues an outbox event (one INSERT, same transaction as finalize); the government API call happens later, in `apps/worker`, never inline with the HTTP request (`docs/architecture.md` §9, brief Rule 12, Scenario L)
- [x] Idempotent: `einvoice_records.sales_document_id` UNIQUE constraint + an in-service Terminal-status check backs it up — a reprocessed outbox event is a safe no-op, tested explicitly (double-processing calls the provider exactly once)
- [x] Unit tests (6 new) + integration tests (6 new: full flow, failed-then-retry, idempotency, outage-doesn't-corrupt-sale, RLS, Ship-to-GSTIN/closure round-trip) — independently re-verified, 200/200 total (was 188), zero real network calls to any government host confirmed by grep across the whole test suite
- [x] Fixture/schema round-trip tests against the mock adapter's canned shapes (IRN, ack, QR, GSTIN, EWB number/validity/Part-B); **no production or sandbox credentials used in the automated test suite** (brief Rule 17) — Scenario J/K's spirit satisfied via the mock adapter per the brief's own "use sandbox/mock adapters" instruction, not by calling the real NIC sandbox
- [ ] **Follow-ups, not done**: `einvoice_provider_credentials` (encrypted-at-rest table) exists but `apps/worker` currently sources sandbox credentials from env vars, not yet reading+decrypting per-legal-entity from that table; only `TAX_INVOICE` auto-enqueues e-Invoice generation (CREDIT_NOTE/DEBIT_NOTE e-invoicing is a real requirement, deferred); `purchases` documents aren't wired through this (only `sales`); e-Way Bill generation is a manually-triggered API call, not auto-enqueued from finalize (deliberate — no goods-movement-threshold business rule exists yet to decide *when* one is legally required, brief Rule 2)

## Stage 9 — Integrations
- [ ] `internal/modules/notifications`: EmailProvider, SMSProvider, WhatsAppProvider interfaces; signed, expiring, revocable share links
- [ ] WhatsApp: official Business Platform integration only, no Web scraping; share logged (who/what/recipient/status/timestamp)
- [ ] API keys: high-entropy, shown once, hashed, scoped, revocable, optional expiry/IP restriction, last-used tracking
- [ ] `internal/modules/webhooks`: HMAC-SHA256 signed events, replay protection, retry/backoff, dead-letter visibility
- [ ] `internal/modules/ai` + MCP server: read-only tool set first (search/get/list per `docs/architecture.md` §11), same authz/audit/rate-limit path as REST, no direct SQL, write tools gated behind explicit policy
- [ ] Scenario L (async resilience) and Scenario M (MCP scope) passing

## Stage 10 — Desktop / Deployment
- [ ] Multi-stage Dockerfiles: `billing-server` (embeds built web SPA), `billing-worker` — non-root, minimal base, read-only FS where practical, healthcheck, graceful shutdown
- [ ] `deploy/compose/docker-compose.yml`: app + postgres by default, optional profiles (redis, minio, reverse-proxy) — reverse proxy explicitly NOT default (`docs/architecture.md` §12)
- [ ] `deploy/casaos/docker-compose.yml`: full `x-casaos` block, amd64+arm64, `docker compose config -q` passing in CI
- [ ] `apps/web`: React/Vite/TanStack Query+Router/RHF/Zod build wired to the API (this is also where Stage 5-9's UI actually gets built — tracked here for the packaging step, UI work itself threads through earlier stages as each module ships)
- [ ] `apps/desktop`: Tauri 2 thin shell around the same web build, zero business logic duplicated
- [ ] Windows packaging prep (MSIX path documented, not signed/submitted yet)

## Stage 11 — Hardening
- [ ] Full security review: IDOR, cross-tenant, RBAC bypass, CSRF, XSS, SQLi, mass assignment, rate-limit bypass, session fixation, webhook replay, API-key scope bypass, MCP permission bypass (brief §69)
- [ ] Load/performance test at 100k products / 1M invoice lines, `EXPLAIN ANALYZE`-driven indexing
- [ ] Concurrency scenarios re-verified at realistic scale
- [ ] Accessibility review (WCAG 2.2 AA) on `apps/web`
- [ ] Backup/restore test — restore into clean environment, verify reconciliation (Scenario N)
- [ ] Migration test against a copy of prior schema
- [ ] All Scenarios A–N passing end-to-end in one continuous run

## Cross-cutting (touched every stage, not a separate phase)
- [ ] `docs/api.md`, `docs/database.md`, OpenAPI 3.1 spec kept current as endpoints ship
- [ ] `CHANGELOG.md`, ADRs for every non-obvious decision
- [ ] CI: gofmt/vet/staticcheck/govulncheck/unit/integration/API/E2E/docker build/compose config on every PR (brief §73)

## Open questions still owed by the user (see `docs/research.md`)
- [ ] e-Invoice/e-Way Bill provider: real GSP/ASP account vs. NIC sandbox placeholder
- [ ] First deployment target: CasaOS (debiancasa) vs. fresh VPS
- [ ] Final product/branding name (repo stays `billing-platform` until then)
