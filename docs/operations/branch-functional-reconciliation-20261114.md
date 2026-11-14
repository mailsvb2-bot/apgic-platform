# APGIC — reconciliation of 47 retired functional branches

Evidence snapshot: `0d21125d0dbc7b1cd21a2a7ca9d28bbffb428f4a` (exact source SHA).

This is a **source-file and named runtime contract reconciliation**, not a
claim that all canon v7 features are RELEASED or external production routes proven.

| Metric | Observed |
|---|---:|
| Archived branch heads, all reachable from main | 47 |
| Branch-changed file versions (branch-specific merge-base) | 520 |
| Identical active file versions | 277 |
| Active files with subsequent revisions | 219 |
| Absent historical file versions, categorized below | 24 |

## Sources inspected

Each archived head in `docs/operations/branch-history-consolidation-20261113.json`
was compared against its individual merge-base with the recorded consolidation
baseline, then with the content of current `main`. The auditable script is
`tools/archived_functional_reconciliation.py`.

## All 47 branches

| Historical head | Changed | Identical | Revised | Absent |
|---|---:|---:|---:|---:|
| `ci/r8-real-provider-proof-on-main-20261105` | 1 | 0 | 1 | 0 |
| `feat/academy-edtech` | 20 | 5 | 15 | 0 |
| `feat/ai-persona` | 29 | 19 | 8 | 2 |
| `feat/client-personal` | 32 | 17 | 15 | 0 |
| `feat/groups-live-events` | 44 | 33 | 9 | 2 |
| `feat/growth-distribution` | 49 | 29 | 16 | 4 |
| `feat/instant-consultation` | 26 | 8 | 17 | 1 |
| `feat/platform-readiness` | 31 | 9 | 19 | 3 |
| `feat/r0-r3-dual-party-consultation-proof-20261019` | 3 | 1 | 2 | 0 |
| `feat/r2-r3-signed-payment-admission-20261026` | 5 | 0 | 3 | 2 |
| `feat/r5-canonical-deletion-state-executor-20261027` | 32 | 18 | 14 | 0 |
| `feat/r5-deletion-lifecycle-core-20261027` | 19 | 0 | 10 | 9 |
| `feat/r5-deletion-request-client-journey-20261027` | 5 | 1 | 4 | 0 |
| `feat/r5-durable-account-deletion-intake-20261027` | 7 | 2 | 5 | 0 |
| `feat/r5-session-confirm-deletion-20261030` | 6 | 4 | 2 | 0 |
| `feat/r8-author-commerce-product-discovery-20261031` | 9 | 4 | 5 | 0 |
| `feat/r8-captured-evidence-all-learning-gates-20261101` | 4 | 3 | 1 | 0 |
| `feat/r8-cohort-integrity-20261031` | 10 | 6 | 4 | 0 |
| `feat/r8-cohort-learner-journey-20261031` | 6 | 4 | 2 | 0 |
| `feat/r8-learner-assessment-results-20261031` | 8 | 3 | 5 | 0 |
| `feat/r8-learner-history-export-20261031` | 4 | 2 | 2 | 0 |
| `feat/r8-learner-verified-access-20261101` | 8 | 2 | 6 | 0 |
| `feat/r8-lesson-schedules-appeals-20261101` | 24 | 18 | 6 | 0 |
| `feat/r8-media-grant-boundary-20261031` | 3 | 2 | 1 | 0 |
| `feat/r8-specialist-unified-workspace-20261101` | 2 | 0 | 2 | 0 |
| `fix/apgic-audit-patches-20261105` | 4 | 3 | 1 | 0 |
| `fix/deletion-residual-guard-20261030` | 3 | 2 | 1 | 0 |
| `fix/ios-staging-dns-transport-retry-20261029` | 1 | 0 | 1 | 0 |
| `fix/r3-signed-payment-room-gate-release` | 4 | 3 | 1 | 0 |
| `fix/r5-client-session-lifecycle-20261026` | 4 | 1 | 3 | 0 |
| `fix/r5-deletion-journey-status-20261029` | 4 | 0 | 4 | 0 |
| `fix/r5-deletion-status-unavailable-personal-20261029` | 2 | 1 | 1 | 0 |
| `fix/r5-personal-history-concurrency-20261026` | 2 | 0 | 2 | 0 |
| `fix/r5-production-deletion-truth-20261026` | 5 | 3 | 2 | 0 |
| `fix/r5-remove-intent-identity-link-20261029` | 2 | 0 | 2 | 0 |
| `fix/r6-r2-durable-slot-exclusion-20261026` | 5 | 3 | 2 | 0 |
| `fix/r6-strict-command-payload-20261026` | 4 | 1 | 3 | 0 |
| `fix/r8-live-workflow-nonempty-main-20261105` | 1 | 0 | 1 | 0 |
| `integrate/cross-lane-assembly-control-current-main-20261112` | 3 | 1 | 2 | 0 |
| `integrate/r10-growth-current-main-20261111` | 47 | 42 | 5 | 0 |
| `integrate/r5-signed-provider-evidence-current-main-20261111` | 3 | 0 | 3 | 0 |
| `integrate/r7-persona-current-main-20261111` | 27 | 25 | 2 | 0 |
| `r8/academy-refund-author-metrics-proof` | 1 | 0 | 1 | 0 |
| `r8/finance-academy-settlement-boundary` | 3 | 0 | 3 | 0 |
| `test/r5-durable-deletion-postgres-journey-20261027` | 2 | 1 | 1 | 0 |
| `test/r8-video-watch-revocation-startup-20261111` | 2 | 1 | 1 | 0 |
| `work/auth001-web-tenant-isolation` | 4 | 0 | 3 | 1 |

## Missing historical paths and their disposition

There are no unclassified missing source paths. Old code is **not** copied
back when it would create a duplicate executor, a collision in applied
migrations, or a redundant testing mechanism. A replacement must be
reassessed if a future change removes the referenced canonical file.

| Former path | Disposition | Active equivalent |
|---|---|---|
| `backend/cmd/deletion-provider-receipt/main.go` | REPLACED_BY_SINGLE_CANONICAL_R5 | `backend/internal/runtimepostgres/account_deletion_provider.go` |
| `backend/cmd/deletion-worker/main.go` | REPLACED_BY_SINGLE_CANONICAL_R5 | `backend/internal/runtimepostgres/account_deletion_runtime.go` |
| `backend/internal/privacy/deletion_evidence_test.go` | REPLACED_BY_SINGLE_CANONICAL_R5 | `backend/internal/privacy/deletion_policy_test.go` |
| `backend/internal/privacy/deletion_provider_receipt.go` | REPLACED_BY_SINGLE_CANONICAL_R5 | `backend/internal/privacy/deletion_provider_attestation.go` |
| `backend/internal/runtimepostgres/account_deletion_executor.go` | REPLACED_BY_SINGLE_CANONICAL_R5 | `backend/internal/runtimepostgres/account_deletion_execution.go` |
| `backend/internal/runtimepostgres/account_deletion_executor_integration_test.go` | REPLACED_BY_SINGLE_CANONICAL_R5 | `backend/internal/runtimepostgres/account_deletion_end_to_end_integration_test.go` |
| `backend/internal/runtimepostgres/account_deletion_revocation.go` | REPLACED_BY_SINGLE_CANONICAL_R5 | `backend/internal/runtimepostgres/account_deletion_provider.go` |
| `backend/migrations/000039_r6_instant_duty_dispatch.sql` | RENUMBERED_SAME_SQL | `backend/migrations/000045_r6_instant_duty_dispatch.sql` |
| `backend/migrations/000042_persona_companion.sql` | RENUMBERED_SAME_SQL | `backend/migrations/000079_persona_companion.sql` |
| `backend/migrations/000043_persona_rights_review.sql` | RENUMBERED_SAME_SQL | `backend/migrations/000080_persona_rights_review.sql` |
| `backend/migrations/000044_r9_live_events.sql` | RENUMBERED_SAME_SQL | `backend/migrations/000074_r9_live_events.sql` |
| `backend/migrations/000045_growth_first_party_attribution.sql` | RENUMBERED_SAME_SQL | `backend/migrations/000075_growth_first_party_attribution.sql` |
| `backend/migrations/000046_growth_integrity_guards.sql` | RENUMBERED_SAME_SQL | `backend/migrations/000076_growth_integrity_guards.sql` |
| `backend/migrations/000047_growth_verified_conversion_bridge.sql` | RENUMBERED_SAME_SQL | `backend/migrations/000077_growth_verified_conversion_bridge.sql` |
| `backend/migrations/000048_growth_owned_link_lifecycle.sql` | RENUMBERED_SAME_SQL | `backend/migrations/000078_growth_owned_link_lifecycle.sql` |
| `backend/migrations/000049_r2_r3_signed_payment_admission.sql` | RENUMBERED_STRONGER_SIGNED_PAYMENT_GUARD | `backend/migrations/000054_r2_r3_signed_payment_admission.sql` |
| `backend/migrations/000049_r5_deletion_reconfirmation.sql` | REPLACED_BY_SINGLE_CANONICAL_R5 | `backend/migrations/000049_r5_deletion_reconfirmation_evidence.sql` |
| `backend/migrations/000050_r5_deletion_erasure_evidence.sql` | REPLACED_BY_SINGLE_CANONICAL_R5 | `backend/migrations/000050_r5_durable_deletion_execution.sql` |
| `docs/operations/integration-control.md` | RETIRED_HISTORICAL_DOCUMENT_NOT_RUNTIME | `historic documentation` |
| `docs/operations/integration-user-journeys.md` | RETIRED_HISTORICAL_DOCUMENT_NOT_RUNTIME | `historic documentation` |
| `docs/operations/launch-v1-assurance.md` | RETIRED_HISTORICAL_DOCUMENT_NOT_RUNTIME | `historic documentation` |
| `docs/r2-r3-signed-payment-admission.md` | RETIRED_HISTORICAL_DOCUMENT_NOT_RUNTIME | `historic documentation` |
| `docs/traceability/R9-live-events-slice.md` | RETIRED_HISTORICAL_DOCUMENT_NOT_RUNTIME | `historic documentation` |
| `tools/auth001_web_tenant_isolation_e2e.sh` | REPLACED_BY_BROWSER_POSTGRES_AUTH001 | `apps/web/e2e/auth001-postgres.spec.ts` |

## Independently exercised user flows (same source snapshot)

- Go backend: `go test ./... -count=1` passed.
- Next.js: production `npm run build` passed.
- Browser journeys: 33 desktop + 33 phone + 33 tablet checks passed,
  covering owned bookings, consultation lifecycle, R5 deletion, R6 instant,
  R8 video revocation and Academy session switch, and specialist marketplace.
- Browser tests include conformance/mock routes and do **not** replace
  real PSP, Yandex OAuth, native store, external provider or production rollout evidence.

## Remaining non-consolidation blockers

- **PR #382** (`fix/r8-live-oauth-secret-scope-20261114`) is a live
  separate head, **not** one of the 47 deleted archived heads; it remains
  Draft pending real OAuth/video and entitlement-revocation acceptance.
- Requirements marked IN_PROGRESS in the canon remain IN_PROGRESS;
  archive reachability and green unit tests do not authorize RELEASED.
- 219 file versions were updated after their archived commits; source
  comparison, public-symbol checks and the exercised journeys bound
  their risk but are not a proof of all untested production behaviors.

No second deletion executor or historical migration sequence was restored.
