# APGIC Academy / EdTech — R8 execution slice

Branch owner: `feat/academy-edtech`. **Never merge into `main` without the user's explicit approval.**

Canonical source: APGIC v7 FINAL §237 (EdTech Learning Domain), §158 (EdTech flywheel), §192 (Academy), §266 (Academy Supply Engine), cross-cutting §§84–91 (Commerce), §228 (Auth), §295 (Legal); Requirement Registry v7 R8 depends on R0 and R2. **The registry does not have a separately named R8 requirement ID: do not fabricate VERIFIED coverage.** Governance changes require RFC.

## Canonical ownership

| Entity | Truth source |
| --- | --- |
| Course → Product/Offer | APGIC PostgreSQL; Product still has distinct commercial owner/author/revenue recipient |
| Version / module / lesson / assessment / attempt | APGIC PostgreSQL |
| Enrollment → specific version → entitlement evidence | APGIC PostgreSQL |
| Progress / completion / certificate / revocation | APGIC PostgreSQL |
| Qualification/evidence candidate | Only `PENDING_VERIFICATION` candidate; separate Qualification owner decides verified status |
| Media bytes and playback attestation | Versioned provider capability, **not** public asset URLs or APGIC Go media streaming |
| Money movement | External PSP/store only; APGIC never handles user funds |

All EdTech entities are scoped to the **same APGIC Identity** as Client/Specialist/Author. No shared runtime DB with ClientPlatform, UCR, BusinessAIOS or VPR. No external/provider-owned Course or Entitlement truth.

## Implemented source files

- `backend/migrations/000032_r8_academy_learning.sql`: relations, immutable version/progress/attempt evidence, verified entitlement guards, cohorts and schedule structural model.
- `backend/internal/academy/`: Go domain contracts and safety invariants.
- `backend/internal/runtimepostgres/academy.go`: atomic writes, transactional outbox, authorization, lesson/activity state, human-graded attempts and review queue, credential verification.
- `backend/internal/httpapi/academy.go`: versioned `/v1/academy/*` API, signed session, safe reason codes, no public raw-lesson bodies.
- `apps/web/app/academy/page.tsx`: author editor, public catalog, reading and assessment flow, instructor queue.
- `apps/mobile/src/mobile-academy-client.tsx`: native catalog, enrolled learner reading/progress and written-assessment submission.
- `contracts/jsonschema/academy-learning-v1.schema.json`: baseline shared schema.
- `backend/internal/httpapi/academy_test.go` and `backend/internal/runtimepostgres/academy_integration_test.go`: unauthorized requests, error privacy, PostgreSQL version and entitlement negative proofs.

## HTTP contracts

| Operation | Method / path | Authorization |
| --- | --- | --- |
| List public courses | GET /v1/academy/courses | Public published catalog, excludes content |
| Author courses | GET /v1/academy/my-courses | Signed author session |
| Create course | POST /v1/academy/courses | Signed product owner of published Product |
| New version | POST /v1/academy/courses/{courseID}/versions | Signed course author |
| Curriculum outline | GET /v1/academy/courses/{courseID}/versions/{versionID}/outline | Signed course author |
| Add module | POST /v1/academy/courses/{courseID}/versions/{versionID}/modules | Signed course author, DRAFT only |
| Add lesson | POST /v1/academy/courses/{courseID}/versions/{versionID}/modules/{moduleID}/lessons | Signed course author, DRAFT only |
| Publish version | POST /v1/academy/courses/{courseID}/versions/{versionID}/publish | Signed product owner/author; immutable afterward |
| Enroll | POST /v1/academy/courses/{courseID}/enroll | Signed learner, verified ACTIVE matching entitlement |
| Resume enrollments | GET /v1/academy/my-enrollments | Signed learner |
| Read lessons | GET /v1/academy/enrollments/{enrollmentID}/learn | Signed learner, active entitlement |
| Confirm reading | POST /v1/academy/enrollments/{enrollmentID}/lessons/{lessonID}/complete | Signed enrolled learner; assessments require reviewed pass; video self-attestation forbidden |
| Submit assessment | POST /v1/academy/enrollments/{enrollmentID}/assessments/{lessonID}/attempts | Signed enrolled learner |
| Instructor review queue | GET /v1/academy/review/attempts | Signed course author; raw student answers stay private |
| Evaluate attempt | POST /v1/academy/attempts/{attemptID}/evaluate | Signed course author; human evidence/rubric |
| Complete & issue certificate | POST /v1/academy/enrollments/{enrollmentID}/complete | Signed learner, all lessons complete and entitlement active |
| Verify credential | GET /v1/academy/certificates/verify/{token} | Public opaque token; no PII, not professional verified |

Signed session is the *only* source of principal. Client-provided identity IDs never authorize. PostgreSQL is authoritative, not Redis/browser. Every successful learning state mutation emits an outbox event in the same DB transaction.

## Fail-closed boundaries

1. No provider-verified ACTIVE `store_entitlements` entry matching `product/{uuid}` → no enrollment, no raw lesson.
2. No signed media provider with genuine playback evidence → video lesson cannot be marked complete. Publishing a course with video lessons is blocked pending the media connector, avoiding an unfinishable paid course.
3. Pending/failed assessment → no assessment completion. Only author-granted human score and evidence can make attempt `PASSED`.
4. Incomplete progress, no entitled access, zero lessons → no certificate.
5. Certificate → **only** qualification evidence candidate with state `PENDING_VERIFICATION`, never an automatic professional credential.
6. Old published course version and learner records never silently rewritten by author edits. New curriculum = new version.

## Gates still needed before claiming full R8 release

- Author/owner **rights and license contracts**, copyright provenance, commercial licensing and royalty reconciliation.
- End-to-end course checkout and provider-agnostic entitlement activation on all allowed rails. **Never simulate a payment.** Existing store-verified entitlement is an intentional integration boundary, not a multi-provider commerce completion.
- Signed media grants, BYOS/managed/external origin delivery, playback attestations; until then video courses remain publish-blocked.
- Operational instructor/cohort enrollment, prerequisites, drip schedules and migration policies (SQL model exists; domain enforcement is not yet implemented).
- Certificate revocation with scoped step-up, audit and appeal; public verifier can read history but no protected revocation API yet.
- Minor/guardian policies, licensing jurisdiction, data export/retention, production monitoring/SLIs, real accessibility/device checks.
- Cross-surface API generated contract binding, reviewed idempotency/concurrency policies and production-like E2E with actual external provider, store policy, UI/browser and native devices.

**Do not mark this area 100%/VERIFIED/RELEASED merely because an API or unit test passes.** R8 is a post-launch capability and every required gate remains explicit until proven by CI and production-like evidence.
