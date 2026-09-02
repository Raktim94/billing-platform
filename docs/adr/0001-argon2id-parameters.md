# ADR 0001: Argon2id password-hashing parameters

Status: Accepted
Date: 2026-09-02

## Context

Brief §27 mandates Argon2id for password storage with a stated floor of
memory ≥ 19 MiB, iterations ≥ 2, parallelism ≥ 1, and asks that the actual
profile be "benchmarked on target hardware."

This benchmark was run inside the development sandbox, **not** on a real
production host — `cpu: QEMU Virtual CPU version 2.5+` per the raw `go test
-bench` output below, i.e. a virtualized/shared CPU of unknown-to-us
provisioning. These numbers are useful for picking a sane default and
understanding the shape of the cost curve, but **must be re-benchmarked on
the actual production host** before go-live, using
`go test ./internal/platform/crypto/... -bench=. -run=^$` — the benchmark
functions already exist in `internal/platform/crypto/password_test.go` for
exactly this purpose.

## Measurements (this sandbox, `benchtime=3x`)

| Memory | Iterations | Parallelism | Latency (ns/op) | Latency (ms) |
|---|---|---|---|---|
| 19 MiB (floor) | 2 | 1 | 16,930,713 | ~16.9 |
| 64 MiB | 3 | 2 | 65,470,681 | ~65.5 |
| 64 MiB | 4 | 4 | 77,750,969 | ~77.8 |
| 128 MiB | 3 | 2 | 136,847,752 | ~136.8 |
| 192 MiB | 2 | 2 | 114,985,247 | ~115.0 |
| 256 MiB | 2 | 2 | 188,764,510 | ~188.8 |

None of these configurations reached the 200–500ms interactive-login target
suggested for a dedicated production host — expected, since this sandbox's
CPU is shared/virtualized and likely slower per-core and more
contended than a real deployment target, and Argon2's cost scales
non-linearly and configuration-dependently, so "which knob to turn" matters
as much as the raw number.

## Decision

Ship **64 MiB memory, 3 iterations, 2 parallelism** (`ARGON2_MEMORY_KIB=65536`,
`ARGON2_ITERATIONS=3`, `ARGON2_PARALLELISM=2`) as the default in
`internal/platform/config`:

- Comfortably above the brief's floor (3.4x the memory floor, 1.5x the
  iteration floor).
- ~65ms on this slow sandbox CPU — on real production hardware (dedicated
  vCPU, no QEMU nesting overhead) this will very likely be faster, which is
  the wrong direction for security margin. This default is deliberately
  configurable via `ARGON2_MEMORY_KIB`/`ARGON2_ITERATIONS`/`ARGON2_PARALLELISM`
  precisely so the deployment operator raises it after benchmarking the
  real host, rather than the code hardcoding a number tuned to a sandbox
  that isn't representative of anything real.
- Parallelism 2 (not 4+) because Argon2id parallelism only helps
  GPU-attacker resistance up to core count, and this platform's login path
  is typically low-QPS (billing-counter operators, not a consumer app) —
  so favoring memory cost over parallelism is the better trade for this
  workload.

## Action required before production go-live

1. Run the benchmark suite on the actual production host(s).
2. If login latency lands under ~150ms, raise `ARGON2_MEMORY_KIB` (memory
   cost has the best security-per-millisecond trade-off of the three
   knobs) until it's in the 200–500ms range, or higher if the operator's
   login QPS is low and they want to prioritize security over
   snappiness — this is a per-deployment operational decision, not a
   code change.
3. Record the new numbers as an addendum to this ADR (or a superseding
   ADR 000X), not a silent config change with no paper trail — brief §60
   requires secrets/crypto configuration to stay auditable.

## Consequences

- Every stored hash embeds its own parameters (`$argon2id$v=19$m=..,t=..,p=..$salt$hash`,
  see `internal/platform/crypto/password.go`), so raising these defaults
  later does not invalidate or require migrating existing password hashes
  — they keep verifying under the parameters they were created with, and
  only get the new (presumably stronger) cost on their next password
  change. No forced-reset migration is needed when this ADR is revisited.
