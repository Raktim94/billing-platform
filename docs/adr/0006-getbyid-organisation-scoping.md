# ADR 0006: every repository `GetByID` filters by `organisation_id`, not just RLS

Status: Accepted
Date: 2026-09-04

## Context

The README's own design-decisions table states: *"RLS as defense-in-depth
— Every tenant table is scoped by `organisation_id` at the application
layer **and** PostgreSQL Row-Level Security — a bug in one layer isn't a
cross-tenant data leak."* A security-review pass (Stage 11 territory,
done ahead of schedule while this environment lacked the Docker/Postgres
needed for the rest of Stage 11) checked whether that's actually true
everywhere, by reading every repository's SQL rather than trusting the
stated intent.

It wasn't. Only `internal/modules/accounting/pg`'s `GetByID` methods
(`accounts`, `journals`, `fiscal_periods`, `bank_accounts`) included
`WHERE organisation_id = $1 AND id = $2`. Thirteen other `GetByID`
methods, across `organisation`, `catalogue`, `contacts`, `pricing`,
`sales`, and `purchases`, did `WHERE id = $1` only — relying entirely on
the RLS session GUC set by `database.Pool.RunScoped` for tenant
isolation, with no application-layer check as a second layer. This is
exactly the single-layer shape the design decision above says should
never exist. It was never exploited (RLS holds, and the existing
integration suite has RLS-itself tests — raw SQL, no org filter in the
query — for exactly this class of bug), but the blast radius of *any*
RLS misconfiguration was strictly larger for every one of those 13
methods than the design intended. This is not hypothetical: Stage 10a's
own history includes exactly this failure mode (the compose file
originally connected the app as the migrator role, which owns the
tables and therefore bypasses RLS entirely — caught by
`WarnIfRuntimeRoleOwnsTenantTables`, not by a second application-layer
check, because at the time none existed for most tables either).

## Decision

Every repository `GetByID` takes `(ctx, orgID, id)` and filters
`WHERE organisation_id = $1 AND id = $2`, with exactly one documented
exception: `organisation.OrganisationRepository.GetByID`, whose sole
caller (`Service.GetOrganisation`) always passes
`principal.OrganisationID` itself as `id` — never a client-supplied
value — so there is no second ID to filter by; the comment at that call
site already explains this ("precisely so a handler cannot be tricked by
a client-supplied ID into fetching another tenant's data").

## Reasoning

1. **Matches the design decision this codebase already committed to in
   writing.** Not a new policy — closing a gap between the stated
   architecture and the actual code.
2. **Cheap and structurally safe to add.** `id` is already a primary key,
   so `AND organisation_id = $1` costs nothing measurable and can't
   change which single row is returned when the row does belong to the
   caller's organisation — only whether a cross-tenant `id` now correctly
   returns "not found" instead of relying on RLS to have already blocked
   the query at the connection level.
3. **Two independently-verified mechanisms beat one.** RLS depends on
   correct role/session-GUC plumbing holding at every call site, forever,
   including future ones. An app-layer `WHERE` clause depends on nothing
   but the query text being what it says it is. Different failure modes;
   both together is what "defense in depth" is supposed to mean.

## Consequences

- Every affected `Repository` interface's `GetByID` signature changed
  from `(ctx, id)` to `(ctx, orgID, id)` — a breaking change to those
  interfaces, absorbed entirely within this commit (all real call sites
  were found by letting the compiler fail, not by grep alone; a stale
  test fake for `pricing.PriceListRepository` needed the same signature
  update to keep implementing the interface).
- **Not independently re-verified against a live database** — this
  environment has no Docker/Postgres to run
  `go test -tags=integration ./...` against. `go build`, `go vet`,
  `gofmt -l`, and every unit test pass, but the integration suite's
  RLS-cross-tenant and per-module RLS sweep tests (which is exactly the
  coverage that would prove this change is correct end-to-end) have not
  run. Run them before treating this as done the way this project's
  other stages define "done."
- `catalogue`'s `UnitOfMeasureRepository`/`CategoryRepository`/
  `BrandRepository` and `organisation`'s `BranchRepository`/
  `WarehouseRepository` `GetByID` methods had no live caller at the time
  of this change (verified — `go build` after the interface change
  compiled cleanly with zero call-site fixes needed for those five) —
  fixed anyway for interface consistency, at zero risk since nothing
  currently calls them.
