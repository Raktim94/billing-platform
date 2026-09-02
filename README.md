# billing-platform

Working name for a multi-company, multi-branch, multi-warehouse billing,
inventory, accounting, and tax-management platform (India GST / e-Invoice /
e-Way Bill capable, extensible to other countries).

Status: **Stage 0/1 — research and architecture**, no application code yet.

Start here:

- [`docs/research.md`](docs/research.md) — verified version facts, open
  questions blocking/shaping later stages.
- [`docs/architecture.md`](docs/architecture.md) — module boundaries, domain
  model, tax/inventory/accounting design, government-integration adapters,
  security architecture, testing strategy, threat model, milestones.

Stack: Go 1.27 (backend, `net/http` + chi), PostgreSQL 18 (pgx/sqlc,
NUMERIC-only for money), TypeScript/React/Vite (web), Tauri 2 (desktop
shell, thin — all business logic stays server-side).

Nothing beyond this scaffold is implemented until the architecture doc is
reviewed and confirmed.
