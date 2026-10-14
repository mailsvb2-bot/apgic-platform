# R4 Financial Completion — integration status

Canonical source: user-supplied `apgic-r4-patches.tar.gz`, prepared against `main` at `d855ef9`.

## Landed
- Dedicated R4 finance release gate (`tools/r4_release_gate.py --gate finance`).
- Required proof types: MARKETPLACE_SPLIT_E2E, PROVIDER_PAYOUT_EVIDENCE, NON_CUSTODIAL_LEDGER_RECONCILIATION, E2E_FINANCIAL_PROOF, RECONCILIATION_PROOF.
- CI-only synthetic manifest entries and fail-closed tests for missing proofs.
- Production gate additionally checks the APGIC-REVENUE-001 Canon dependency. CI synthetic evidence cannot authorize production.

## Not landed yet from the supplied patch series
- Append-only PostgreSQL finance migration and invariant checks.
- Settlement / payout / reconciliation runtime, worker and signed webhook.
- Finance status API, OpenAPI/TypeScript contract and order status screen.
- Requirement registry implementation/test references and integration test suite.

These must be integrated and tested before this branch can be merged. The original patch series reports known gaps: no native status screen, incomplete demand-source attribution and webhook sequencing, no discrepancy resolution workflow, no staging finance worker, and no real provider certification.

**Release state: IN_PROGRESS; not production-ready.** Do not mark requirements VERIFIED or RELEASED based on CI-only proof.
