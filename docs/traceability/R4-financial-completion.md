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

## Release blockers
- No native iOS/Android order finance status screen.
- Demand-source attribution not fully modeled on orders; marketplace case only / otherwise fail closed.
- No consultation.completed event; worker polls completed consultations.
- Provider event stream sequencing absent beyond event ID + digest idempotency.
- Finance discrepancies have no resolution workflow/admin interface.
- Finance tables not yet part of readiness requiredTables; staging finance-worker service has not been installed or proven.
- No actual PSP certification, real payout split/reconciliation or documented production evidence.

## Decision
**Development integration is not a production release.** CI and PostgreSQL proof must pass on the exact branch head. Do not mark finance requirements VERIFIED/RELEASED or merge on the basis of synthetic CI gates.
