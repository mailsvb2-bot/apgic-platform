# R1 Specialist Supply traceability

This document records the executable trace for the first Specialist Supply vertical slice.

## APGIC-ID-001 — Single identity with multiple roles

- Contract: `contracts/openapi/apgic-v1.yaml` — specialist profile bootstrap reuses or creates the signed session Identity.
- Implementation:
  - `backend/internal/httpapi/specialist.go`
  - `backend/internal/runtimepostgres/specialist.go`
  - `backend/internal/runtimepostgres/identity_roles.go`
- Tests:
  - `backend/internal/httpapi/specialist_test.go`
  - `backend/internal/runtimepostgres/specialist_test.go`
- Evidence:
  - CI Go race/unit suite.
  - PostgreSQL integration step `TestSpecialistSupplyPersistsEvidenceReviewAndPublishGate`.

## APGIC-SPEC-001 — Capability evidence states

- Contract:
  - `contracts/openapi/apgic-v1.yaml`
  - `packages/contracts/src/generated/apgic-v1.ts`
- Persistence:
  - `backend/migrations/000007_r1_marketplace_discovery.sql`
  - `backend/migrations/000019_r1_specialist_supply_runtime.sql`
- Implementation:
  - `backend/internal/specialist/model.go`
  - `backend/internal/runtimepostgres/specialist.go`
- Required state progression:
  - declaration: `SELF_DECLARED / NOT_APPLICABLE`
  - evidence intake: `DOCUMENT_SUPPORTED / PENDING`
  - trusted review only: `APGIC_VERIFIED / ACTIVE`
- Public API exposes no verification/review mutation.
- Tests:
  - `backend/internal/runtimepostgres/specialist_test.go`
  - `backend/internal/httpapi/specialist_test.go`
  - `apps/web/e2e/foundation.spec.ts`

## APGIC-SPEC-002 — Supply-Min publish gate

- Persistence:
  - canonical qualification and publication tables/guards in `backend/migrations/000007_r1_marketplace_discovery.sql`
- Implementation:
  - `backend/internal/runtimepostgres/specialist.go`
  - `backend/internal/marketplace/qualification.go`
  - `backend/internal/marketplace/discovery.go`
- Gate:
  - profile must be complete;
  - profile review must be `APPROVED`;
  - requested capability must be `APGIC_VERIFIED / ACTIVE`;
  - qualification policy must return eligible;
  - otherwise publication fails closed with a reason code.
- Tests:
  - `backend/internal/runtimepostgres/specialist_test.go`
  - `apps/web/e2e/foundation.spec.ts`
- Database guard:
  - `specialist_publications_guard` prevents bypass of the qualification/profile gate.

## Web surface

- Entry: `apps/web/app/specialist/page.tsx`
- Stateful onboarding: `apps/web/app/specialist/onboarding.tsx`
- Same-origin API proxy: `apps/web/app/v1/[...path]/route.ts`
- The page does not claim that submitted evidence is verified before trusted review.
