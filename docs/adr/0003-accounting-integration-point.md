# ADR 0003: How sales/purchases post to accounting

## Status

Accepted (Stage 6).

## Context

docs/architecture.md §7 says every module with a financial effect "calls
`accounting.Post(ctx, JournalRequest)`" but doesn't specify exactly *where*
in the calling module's code that happens, or which side (accounting vs.
the caller) decides which GL accounts a given business event maps to.

`sales.FinalizeDocument` and `purchases.FinalizeDocument` already open one
`RunScoped` transaction each and call into other modules from inside it
(`taxation.CalculateAndSnapshotTx`, `inventory.RecordMovementForOtherModule`)
for atomicity — the same shape `accounting.PostTx` needed to follow.

## Decision

1. **Where**: `accounting.PostTx` is called from inside
   `sales.FinalizeDocument`'s and `purchases.FinalizeDocument`'s *existing*
   `RunScoped` block, immediately after the tax snapshot / stock movement
   steps already in that transaction — not a second transaction, not an
   outbox event. A sale's finalized status, its tax snapshot, its stock
   deduction, and its accounting journal all commit or roll back together.
   This mirrors `CalculateAndSnapshotTx`/`RecordMovementForOtherModule`'s
   existing nested-transaction-safe pattern exactly.

2. **Who decides the account mapping**: the *calling* module (sales,
   purchases), not accounting. `accounting` exposes generic
   `JournalRequest`/`PostTx` plumbing plus exported default-chart account
   code constants (`domain.CodeAccountsReceivable`, `domain.CodeSales`,
   etc.); sales/purchases construct the specific Dr/Cr lines because *they*
   are the modules that know what a "tax invoice" or "purchase invoice"
   means in accounting terms. `accounting` only knows "here is a balanced
   set of lines, post them" plus enforcing the three-layer invariant and
   fiscal-period locking — it has no per-source-type posting rules baked in.
   This keeps `accounting` a mechanism, not a policy engine for every
   document type this system will ever have.

3. **Which document types post a journal**: not every finalized document.
   - Sales: `TAX_INVOICE`, `POS_INVOICE`, `SALES_RETURN`, `CREDIT_NOTE`,
     `DEBIT_NOTE`, `RECURRING_INVOICE` — real billing/revenue events.
     `QUOTATION`, `PROFORMA_INVOICE`, `SALES_ORDER`, `DELIVERY_CHALLAN` do
     NOT post — they're commitments or goods-movement documents with no
     revenue recognized yet (a new `RevenueAffecting(DocumentType) bool` in
     `sales/domain`, mirroring the existing `StockAffecting` classification
     pattern).
   - Purchases: `PURCHASE_INVOICE`, `PURCHASE_RETURN`, `DEBIT_NOTE` post
     (the existing code's own comment already called these "billing
     documents whose accounting effect is Stage 6 scope"). `PURCHASE_ORDER`
     and `GOODS_RECEIPT` do not — a GRN records physical receipt of goods
     at a cost (already captured in `stock_balances.average_cost` since
     Stage 4), not a confirmed supplier bill; the payable is booked when
     the actual invoice is finalized, avoiding double-counting if the GRN's
     received cost and the invoice's billed amount ever differ.

4. **Sales posting** (uses the tax snapshot sales already computes):
   `Dr Accounts Receivable (party-tagged) [grand total] / Cr Sales [taxable
   amount] / Cr GST Output Tax Payable [tax amount]`. `SALES_RETURN`
   reverses the debits/credits. Discount is already netted into each
   line's `LineTotal` before tax calculation (Stage 5b), so it is not
   separately booked to the `Discounts` account in this pass — a full
   gross-sale-plus-contra-discount presentation is a legitimate refinement,
   not built here.

5. **Purchases posting — a deliberate simplification, not full ITC
   tracking**: `purchases` has never called the tax engine (unlike `sales`,
   it has no `TaxDocumentID`, no GSTIN/state-code wiring to the supplier's
   tax registration). Building that properly — resolving the supplier's
   state from `contacts.TaxRegistration`, calling `taxation.CalculateAndSnapshotTx`
   the same way `sales` does, splitting the posted amount into `Dr
   Purchases` + `Dr GST Input Tax Credit` — is real, correct, buildable
   work, but it's a second significant integration (wiring an entire
   module through the tax engine for the first time) on top of an already
   large Stage 6 pass. Given this session's explicit priority order
   (3-layer invariant → auto-posting atomicity → Scenario E → ledger →
   fiscal periods), and that Scenario E is sales-only, purchases posts the
   simpler `Dr Purchases [full line-total sum, tax-inclusive as entered] /
   Cr Accounts Payable (party-tagged) [same amount]` — no separate ITC
   line. **This is a flagged follow-up, not a silent gap**: a business
   using this system today gets a correct payable balance and a correct
   (if not GST-input-credit-split) expense figure; full ITC tracking needs
   purchases wired through `taxation`/`gstindia` the same way `sales` is,
   which is a bounded, well-precedented next step (the pattern to copy
   already exists verbatim in `sales.FinalizeDocument`).

6. **Chart-of-accounts seeding is not wired into `identity.Bootstrap`**.
   `accounting.EnsureDefaultChartOfAccounts` is idempotent and callable
   per-organisation, but calling it automatically when a new organisation
   is created would mean `identity` (Stage 2, foundational) importing
   `accounting` (Stage 6) — a layering inversion docs/architecture.md §2
   argues against (lower-numbered, more foundational modules shouldn't
   depend on higher-numbered ones). The layering-safe place to chain
   "create organisation, then seed its chart of accounts" is
   `apps/server`'s composition root, which is allowed to depend on every
   module — this wiring is a small, low-risk follow-up, not done in this
   pass since it touches the bootstrap HTTP handler rather than being pure
   new-module work.

## Consequences

- A business whose deployment never calls `EnsureDefaultChartOfAccounts`
  for its organisation will get a clear, actionable error ("resolving
  account 1100: not found — has EnsureDefaultChartOfAccounts run for this
  organisation?") the first time `sales.FinalizeDocument` tries to post,
  rather than a silent skip or a cryptic FK violation.
- Extending purchases to full ITC tracking later is additive: a new
  `taxation`/`gstindia` call in `purchases.FinalizeDocument`, a new posting
  branch splitting the amount, no schema change to `accounting` itself
  (the generic `JournalRequest`/`Post` plumbing doesn't care how many lines
  a caller sends).
