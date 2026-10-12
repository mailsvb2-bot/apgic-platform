# APGIC — ten-lane integration decision protocol

**Authority:** Canon v7 §226 (Requirement traceability), §240–241 (versioned commands/events), §304–305 (single truth + cross-surface conformance), §306–310 (RELEASED distinct from VERIFIED; R0–R4 cut/dependencies/evidence), §314–315 (release governance). **Owner of coordination:** `feat/platform-readiness`. **Canonical business owners** remain those of the respective requirements. No donor repository is changed. This protocol does not create a second product registry or change the frozen Canon.

## What was verified on GitHub (12 Oct 2026)

Compared the **ten** `feat/*` lane heads against `main` `4cf51b93070fb321799e0b0c296c66afcd920b8b`, without merging anything. Source: GitHub branch comparisons, captured as part of each integration report using full 40-character heads. All ten were separate work streams; their individual green CI results cannot establish joint functional compatibility.

- **PostgreSQL:** `000032` was independently allocated by eight lanes: specialist marketplace, client personal, consultation runtime, persona, instant consultation, academy, live events and growth. Client personal also introduced `000033`; Growth also introduced `000033` and `000034`. **Blocker owner: `postgres-schema-owner`, with each lane responsible for reviewing its DDL.** File permalinks are generated in the JSON report. The proposal in `config/release/integration-plan-v1.json` is only a provisional allocation for the eventual **new integration candidate**; it does not mutate any source branch or establish that renaming is safe.
- **Public OpenAPI:** `contracts/openapi/apgic-v1.yaml` changed independently in specialist, booking and client personal. **Owner: `api-contract-owner`.** Before composing web/mobile, reconcile paths, command semantics, idempotency, `ErrorEnvelope`, auth scopes and backward compatibility. Regenerate `packages/contracts/src/generated/apgic-v1.ts` exactly once from the reconciled OpenAPI. **Never hand-merge generated clients.** Other lanes with new JSON Schemas still require references/compatibility checks to the consolidated contract.
- **Backend composition:** `backend/internal/httpapi/handler.go`, `backend/cmd/api/main.go`, PostgreSQL `check.go` and related runtime composition overlap. **Owners: `http-composition-owner`, `runtime-composition-owner`, `postgres-runtime-owner`.** No duplicate domain truth: a route handler is a transport adapter, not a second owner of booking/payment/qualification.
- **Web/native:** `apps/web/app/journey.tsx`, `apps/mobile/src/App.tsx`, common navigation and CSS changed across streams. **Owners: `web-journey-owner`, `native-shell-owner` and `design-system-owner`.** Reconcile one end-to-end route first; do not choose whichever screen happens to build.

## Merge order and dependency proofs

This is the **reviewed sequence for the future isolated integration candidate**, *not* permission to merge into `main` or author changes in other lane branches. A separate integration candidate/ref must be selected and pinned by exact SHA; the current readiness branch is not an integrated candidate. Each stage preserves migrations, compiled Go, generated web/native clients and the previous stage's business outcome.

| Phase | Owning direction | Required observed outcome before advancing |
|---|---|---|
| R0 | Existing `main` foundation + `platform-readiness` coordination | Identity → tenant-denial/consent → durable outbox/audit; isolated PostgreSQL restore and web/iOS/Android baseline |
| R1 | `feat/specialist-marketplace` | Eligible specialist published → qualification → discovery → bookable public offer, with revoked/ineligible negative path |
| R2 | `feat/booking-complete` | Real offer → slot/hold → order/payment provider → authoritative booking + entitlement → notification; double-book, retry and refund failures |
| R3 | `feat/consultation-runtime` | Entitled booking → ready/join → CommunicationProvider signed lifecycle → completed outcome + recovery/reconnect, including mobile lifecycle |
| R4 | Financial/provider owners + `platform-readiness` | Commission/beneficiary + provider payout/settlement receipt → immutable ledger reconciliation, native distribution evidence; **not satisfied by mock checkout** |
| After Launch v1 | `feat/client-personal`, `feat/ai-persona`, `feat/instant-consultation`, `feat/academy-edtech`, `feat/groups-live-events`, `feat/growth-distribution` | Integrate in the explicit `integration-plan-v1.json` order **after their upstream contracts**; R6/R8/R9 and Growth are **not** new R0–R4 obligations |

The preceding table concerns **integration order**, not current completion percentages. The actual requirement registry/evidence remains the authority for release statuses. A branch containing 1 or 50 commits is not inherently `VERIFIED`.

## PostgreSQL merge protocol (no blind renumbering)

1. Freeze the exact heads and derive the migration DAG / object dependencies from all files, including existing `000001`–`000031` and the `ci_*` invariant checks. A foreign key or backfill dependency overrides the provisional lane order.
2. Review names, DDL constraints, ownership, PK/FK, exclusions, money/time semantics, test fixtures and any hardcoded migration version references. Reconcile one global monotonic sequence in the **integration candidate only**. If a migration has already reached any persistent DB, use forward corrective migrations instead of renaming applied history.
3. On a disposable database that actually matches production migration history, execute the consolidated forward chain and verify checksums/versions, tenant isolation, unique reservations, immutable ledger, outbox idempotency, provider signatures, rights/consent and concurrency. Also run from a clean database and a representative restored snapshot; record exact candidate, PostgreSQL version, applied migration ledger, runtime config, test/SQL evidence and operator.
4. Verify migration restart/idempotency, deployed old/new API overlap, rolling update, rollback limitations and a recoverable failure case. Never recover financial integrity by undoing committed ledger events.
5. Record the **actual** integrated migration allocations as evidence; change the proposal via reviewed decision, not silent editing.

## API and surface agreement

The canonical owner of `contracts/openapi/apgic-v1.yaml` reviews all proposed endpoints and reconstitutes a single versioned OpenAPI with request/response schemas. Check required/optional field semantics, stable IDs, ErrorEnvelope, optimistic versions, `Idempotency-Key`, timezone-aware instants, amount_minor/ISO currency, tenant scope and provider-neutral error codes. Regenerate clients once, then run Go ↔ generated web ↔ native compatibility against **the same API image**, not branch-local servers. Connector provider SDKs stay behind APGIC connectors; external systems cannot own APGIC order or payment truth.

## One release assessment, two blocker scopes

Run the owner/blocker collector **without merging**:

```bash
python tools/integration_release_control.py \
  --candidate-ref feat/platform-readiness \
  --output evidence/integration-branch-control.json \
  --summary /tmp/apgic-integration-summary.md
```

It reads live GitHub branch heads, comparisons, candidate ancestry, migration numbering and common contract/composition paths. It **never changes a branch**. JSON includes `decision`, `candidate_sha`, `launch_blockers[]` and `post_launch_blockers[]`. Every blocker has `code`, an **owning direction/role** (not an invented person), affected branches and pinned source links. When missing evidence is the blocker, the report says missing; an absent record is **not proof of a failed production action**. Unavailable GitHub data fails closed. `--snapshot` accepts a pinned reproducible offline JSON snapshot of the same shape.

A **separately generated** `tools/launch_readiness.py --strict` assessment may be supplied via `--launch-assessment`. This reuses the frozen Canon Registry, R0–R4 cross-cutting requirements, actual production journey proof, native distribution and provider financial evidence. A self-declared CI pass or a marker named “verified” is not a substitute. The integration collector never sets `promotion_authorized=true` and it is not authorized to deploy.

An integrated release candidate can be **ready for separate promotion review** only if all mandatory branch heads are actually ancestors of its exact SHA, no launch-scoped merge/schema/contract conflict remains, and its existing exact-SHA strict Launch v1 assessment passes. A passing assessment is still **not** `RELEASED`; a separate authorized rollout, canary/business probe and observed production outcome are required. If any user/operational journey fails, report concrete stage, owner, actual observation/trace, external provider receipt where applicable, negative case and blocker; never launder staging into production.

### Release acceptance evidence matrix

| Journey ID | Owner | Required trace / negative proof |
|---|---|---|
| `r0-tenant-denial` | platform-core | Cross-tenant request denied, audit correlation, no state leakage |
| `r0-outbox-recovery` | platform-core | Committed outbox retried with same event, no duplicate side effect |
| `r0-isolated-restore` | platform-operations | Restored independent PostgreSQL with measured RPO/RTO, verified ledger/tenant integrity |
| `r1-eligible-discovery` | specialist-marketplace | Eligibility → offer, same web/native semantics; revoked profile not bookable |
| `r2-booking-payment-entitlement` | booking-complete | Provider-authentic callback → order/booking/entitlement trace and duplicate/timeout paths |
| `r2-refund-reversal` | booking-complete | Cancel/refund/reversal receipts, compensating ledger, no second payout |
| `r3-consultation-outcome` | consultation-runtime | Signed join/completion and reconnect/failure state, web/iOS/Android |
| `r4-provider-settlement` | finance-provider-integrations | Provider-executed payout + reconciliation against noncustodial ledger |
| `r4-native-store` | mobile-release-operations | Distribution/signing/provider-store and native release/runtime evidence |

**Current state:** release remains BLOCKED: conflicting migrations, uncomposed critical API/code/client changes and absent an integrated end-to-end candidate/evidence. Branch-specific ownership and source files are verifiable. Exact individual operator names have not been appointed in the source and must be assigned explicitly before remediation can be considered owned.
