# R4 Financial Completion — integrated implementation and release state

Source: user-provided `apgic-r4-patches.tar(1).gz` (five ordered patches; originally based on `main` at `d855ef9`). Changes integrated in branch `feat/r4-financial-completion` and proposed through PR #255.

## Implemented in this branch
- PostgreSQL migration `000039_r4_financial_completion.sql`: append-only financial records and invariant triggers. Fixture and invariant checks in `backend/migrations/ci_r4_finance_*.sql`.
- `backend/internal/runtimepostgres/finance.go`: settlement of completed consultations, hold decisions, commission snapshots, provider execution instructions, payout evidence processing, dispute effects and replay-safe reconciliation.
- `backend/cmd/finance-worker`: settlement/reconciliation worker with tests.
- `backend/internal/httpapi/finance.go`: Ed25519-verified provider webhook ingress and authorized order finance status endpoint, with tests.
- OpenAPI contract and generated TypeScript client, `/orders/[id]/finance` web route.
- R4 release gate with five explicit finance evidence classes; synthetic CI evidence is not production evidence.
- Requirement links for R4 finance remain **IN_PROGRESS** until verifiable external-provider and staging/production proof exists.

## Evidence-based completion assessment (2026-10-15)

The following classification separates **code present on this branch** from **verified production behavior**. Presence is not provider certification.

| Capability | Implementation / evidence in this branch | Remaining acceptance gate |
| --- | --- | --- |
| Canonical settlement, commission snapshots and provider payout evidence | `backend/internal/runtimepostgres/finance.go`; `backend/cmd/finance-worker`; migration `000039`; PostgreSQL integration and invariant tests | End-to-end signed provider execution, provider certification and financial reconciliation in staging/production |
| Native order finance status | `apps/mobile/src/native-finance-status.tsx`, `mobile-finance-status-client.ts` and tests | Installed Android/iOS order-specific E2E with authenticated ownership checks and actual provider facts |
| Provider stream ordering | `backend/migrations/000041_r4_finance_provider_stream_sequence.sql`; sequenced provider intake and replay/gap checks in `finance_integration_test.go` | Real provider signed webhook stream, retries/reordering and restart evidence |
| Discrepancy resolution | Migration `000040`, `finance_resolution.go`, audited and step-up-protected `finance_operations.go`, Web operations queue | Verified production step-up ceremony, operator workflow and HTTP/PostgreSQL end-to-end negative and recovery tests |
| Operator capability grants | `backend/cmd/finance-operator-admin`, grant/revocation audit tests, `docs/operations/finance-operator-grants.md` | Authorized operator onboarding, revocation and separation-of-duties acceptance |
| Database schema readiness | `backend/internal/runtimepostgres/check.go` includes R4 finance tables in `requiredTables`; migration/invariant CI | Staging deployment readiness, schema/worker startup and restart proof |
| Finance worker deployment | `deploy/staging/apgic-finance-worker-staging.service` and opt-in staging deployment configuration | Actual installed/running service and provider-backed worker outcome evidence |
| Demand-source commission eligibility | Fail-closed policy when source or policy is unsupported | Prove each supported source attribution and immutable order snapshot through signed payment and settlement |
| Consultation completion trigger | Worker scans completed consultations | Prove durable event-driven completion, wake-up/replay and exactly-once settlement across worker restarts |
| Native financial deep-link | Native finance status component and mobile routing code exist | Device-level deep-link → authorized order → current finance result; revoked or foreign owner denied |
| External money movement | APGIC stores non-custodial facts/instructions, never customer funds | Independent bank/PSP/store certification, settlement/payout/refund/chargeback proof and release approval |

### Non-negotiable release decision

Code review, unit tests, PostgreSQL fixtures and a green GitHub Actions run are **development evidence**, not actual payment, payout or provider certification. Do not mark R4 requirements `VERIFIED` or `RELEASED` without the specified signed external evidence and device/worker journeys. Keep R4 Draft until these gates are independently demonstrated. Do not change its scope by importing donor business truth, making APGIC custodial, or hiding unresolved provider discrepancies.

## Decision
**Development integration is not a production release.** CI and PostgreSQL proof must pass on the exact branch head. Do not mark finance requirements VERIFIED/RELEASED or merge on the basis of synthetic CI gates.

## R4 Part 2 — discrepancy resolution and native financial status

- `backend/migrations/000040_r4_finance_discrepancy_resolution.sql`: append-only operator grants and revocations, typed single-resolution records, same-provider correction evidence and later matched reconciliation safeguards.
- `backend/internal/runtimepostgres/finance_resolution.go`: idempotent discrepancy resolution, open queue, canonical audit/outbox events and order-status projection.
- `backend/internal/httpapi/finance_operations.go`: audited, step-up-protected operator endpoints (queue and resolution), server-bound identity and session.
- `contracts/openapi/apgic-v1.yaml` and generated TypeScript: financial operations contracts and resolution status.
- `apps/mobile/src/native-finance-status.tsx`, `mobile-finance-status-client.ts` and tests: fail-closed, non-cached owner-scoped financial status.
- Postgres invariant probes and integration tests cover missing/revoked grants, stale evidence, invalid kind, duplicate/conflicting resolution, append-only rejection and successful resolution.

The requirements remain **IN_PROGRESS**. The table above is the current evidence-based backlog; provisioning CLI and webhook sequencing are already implemented, but operational certification and the named end-to-end proofs are outstanding.


## Operator grant provisioning

`backend/cmd/finance-operator-admin` now provisions and revokes `FINANCE_DISCREPANCY_RESOLVE` grants through explicit CLI commands. Grant/revocation and audit evidence are written atomically; the tool requires approval metadata and refuses duplicate active grants. See `docs/operations/finance-operator-grants.md`. The web discrepancy queue is read-only until the production step-up ceremony is connected.
