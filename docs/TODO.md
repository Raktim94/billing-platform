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

## Stage 5b — Sales Documents & Printing
- [ ] `internal/modules/sales`: quotation, proforma, sales order, delivery challan, tax invoice, credit/cash invoice, credit/debit note, sales return, recurring invoice, POS invoice, conversion flows (estimate→invoice, order→invoice, challan→invoice)
- [ ] Invoice header + line fields per brief §5, calculation snapshot immutability (brief §55) — calls Stage 5a's `TaxEngine`, calls `inventory.RecordMovementForOtherModule` on finalize (SALE/SALE_RETURN movement types already exist from Stage 4)
- [ ] Document numbering: configurable series (organisation/legal-entity/branch/FY-scoped per brief §51), concurrency-safe allocation (Scenario I) — the real, full numbering system (purchases' Stage 4 counter was explicitly a minimal placeholder, not this)
- [ ] Printing/PDF template engine: A4 GST invoice, compact, thermal 80mm/58mm, quotation, PO, challan, receipt, statement, credit/debit note (brief §19)
- [ ] Sales screen UX groundwork (API side): fast product/customer search, stock/price/tax visibility in one call
- [ ] Unit + integration tests

## Stage 6 — Accounting
- [ ] `internal/modules/accounting`: chart_of_accounts, journals, journal_lines, fiscal_periods, payments, receipts, bank/cash accounts, reconciliation
- [ ] Double-entry invariant enforced at 3 layers (app check, deferred DB trigger, no-UPDATE grant on posted rows) per `docs/architecture.md` §7
- [ ] Auto-posting from finalized sales/purchase documents and payments
- [ ] Customer/supplier ledger: chronological, running balance, ageing buckets, credit limit/overdue alerts
- [ ] Fiscal year config + period locking (brief §52)
- [ ] Property tests: Σdebit == Σcredit for any valid journal (brief §65)
- [ ] Scenario E (partial payment/outstanding balance) passing end-to-end

## Stage 7 — Reports / Dashboard
- [ ] Sales, inventory, purchase, accounting, tax report datasets per brief §22 (full list)
- [ ] GSTR-1-oriented and GSTR-3B-oriented export preparation (explicitly NOT filing — brief §8)
- [ ] Export: XLSX/CSV/PDF/JSON, background job + expiring download link for large exports
- [ ] Dashboard cards + charts (brief §23), materialized/aggregate query design — no raw full-table scans per refresh
- [ ] Report filters: date/FY/branch/warehouse/GSTIN/customer/product/HSN/salesperson/tax rate/doc type/payment status

## Stage 8 — Government Integrations (sandbox only)
- [ ] `internal/modules/einvoice`: `EInvoiceProvider` interface, `v1` adapter against NIC sandbox (einv-apisandbox.nic.in), persist IRN/ack/signed QR/status per `docs/architecture.md` §9
- [ ] `internal/modules/ewaybill`: generate/retrieve/cancel/Part-A/Part-B/vehicle+transporter update/validity extension, **Ship-to GSTIN + voluntary closure fields** (2026-08-01 GSTN change, `docs/research.md`)
- [ ] Explicit status machine (DRAFT→QUEUED→SUBMITTING→GENERATED|FAILED_RETRYABLE|FAILED_FINAL, CANCEL_PENDING→CANCELLED, CLOSED), outbox-driven, idempotent
- [ ] Never call from sales domain code directly — adapter boundary enforced (`docs/architecture.md` §2)
- [ ] Fixture tests against official/sandbox schema; **no production credentials in CI** (brief Rule 17)
- [ ] Scenarios J/K passing against sandbox

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
