# ADR 0004: Dashboard query design — live indexed queries, not a materialized table

## Status

Accepted (Stage 7).

## Context

Brief §23 explicitly warns against the dashboard "scan[ning] millions of
raw rows for every dashboard refresh," and requires "optimized aggregate
queries/materialized reporting tables where needed" — leaving the actual
mechanism (live query vs. materialized table) to the implementer.

The dashboard summary (`internal/modules/reporting/domain.DashboardSummary`)
needs eight figures: today's sales, today's collections, today's
purchases, outstanding receivable, outstanding payable, current stock
value, low stock count, overdue receivable.

## Decision

Use **live, parameterized SQL queries against a small, deliberately
bounded set of indexed tables**, issued as one batched round trip (a
single `SELECT` with several scalar subqueries) rather than eight
independent requests — not a periodically-refreshed materialized summary
table.

Two composite indexes were added specifically for this stage's hot paths
(migration 0022): `sales_documents(organisation_id, status, issue_date)`
and `purchase_documents(organisation_id, status, document_date)`. Every
other card either reads `stock_balances` (already primary-keyed on
`(organisation_id, warehouse_id, product_variant_id)`, and its row count
is bounded by catalogue size — distinct warehouse×variant pairs — not
transaction volume, so it stays small and fast without a dedicated index
even at meaningful transaction scale) or reuses `journal_lines`' existing
`(organisation_id, account_id)`-shaped indexes from Stage 6.

## Why not a materialized table

- **Staleness is a real cost, not a free win.** "Today's Sales" showing a
  number that's 5-15 minutes stale (a typical materialized-view refresh
  cadence) is a worse experience for a billing-counter dashboard than a
  live query, unless the live query is measurably too slow to run on
  every load — which it isn't yet (see Measured, below).
- **No refresh infrastructure exists yet.** `apps/worker` and a scheduled-
  job mechanism are Stage 9+ territory (the outbox/background-job system
  brief §34/§54 describes). Building a one-off `REFRESH MATERIALIZED VIEW`
  cron path just for this dashboard, ahead of that general mechanism,
  would be infrastructure built twice.
- **Self-hosted, single-business scale.** This system's primary deployment
  target (docs/architecture.md §12, `docs/research.md`) is one business's
  self-hosted instance — realistically thousands to tens of thousands of
  documents, not the 1M-invoice-line brief §70 explicitly reserves for
  Stage 11's dedicated performance-testing pass at deliberately large
  synthetic scale.

## Measured

Stage 7's integration test seeds ~500 sales documents, ~200 purchase
documents, and a few thousand stock/journal rows, then calls the
dashboard-summary endpoint and asserts it completes in under 200ms on
this development machine (see `tests/integration/reporting_test.go`,
`TestReporting_Dashboard_PerformanceSanityCheck`) — a sanity check at
moderate scale, not the brief §70 100k/1M-row benchmark, which is
explicitly Stage 11 scope.

## Revisit if

Stage 11's real 100k-product/1M-invoice-line performance pass
(brief §70) shows the live-query approach doesn't hold at that scale.
The fallback is a materialized summary table refreshed by `apps/worker`
on a short interval (e.g. every 1-5 minutes) once that infrastructure
exists — this ADR's queries would become the materialized view's
definition rather than being thrown away.
