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

- `backend/migrations/000060_r8_academy_learning.sql`: relations, immutable version/progress/attempt evidence, verified entitlement guards, cohorts and schedule structural model.
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
| Learner attempt outcomes | GET /v1/academy/enrollments/{enrollmentID}/attempts | Signed owner of enrollment; grading status/score only; no instructor evidence |
| Instructor review queue | GET /v1/academy/review/attempts | Signed course author; raw student answers stay private |
| Evaluate attempt | POST /v1/academy/attempts/{attemptID}/evaluate | Signed course author; human evidence/rubric |
| Complete & issue certificate | POST /v1/academy/enrollments/{enrollmentID}/complete | Signed learner, all lessons complete and entitlement active |
| Verify credential | GET /v1/academy/certificates/verify/{token} | Public opaque token; no PII, not professional verified |

Signed session is the *only* source of principal. Client-provided identity IDs never authorize. PostgreSQL is authoritative, not Redis/browser. Every successful learning state mutation emits an outbox event in the same DB transaction.

## Fail-closed boundaries

1. No provider-verified ACTIVE `store_entitlements` entry matching the canonical product UUID → no enrollment, no raw lesson.
2. No signed media provider with genuine playback evidence → video lesson cannot be marked complete. Publishing a course with video lessons is blocked pending the media connector, avoiding an unfinishable paid course.
3. Pending/failed assessment → no assessment completion. Only author-granted human score and evidence can make attempt `PASSED`.
4. Incomplete progress, no entitled access, zero lessons → no certificate.
5. Certificate → **only** qualification evidence candidate with state `PENDING_VERIFICATION`, never an automatic professional credential.
6. Old published course version and learner records never silently rewritten by author edits. New curriculum = new version.

## Gates still needed before claiming full R8 release

- Author/owner **rights and license contracts**, copyright provenance, commercial licensing and royalty reconciliation.
- End-to-end course checkout and provider-agnostic entitlement activation on all allowed rails. **Never simulate a payment.** Existing store-verified entitlement is an intentional integration boundary, not a multi-provider commerce completion. The academy now uses the canonical **bare product UUID** stored in `store_transaction_verifications.product_ref` and `store_entitlements.product_ref` (not `product/{uuid}`), and confirms the captured payment effect + verification identity. An isolated PostgreSQL CI journey exercises the existing synthetic, verification-backed store transaction through authoring → publication → entitlement → learning → human review → certificate, including two storage/process restarts. **This is sandbox evidence, not a claim of a live production sale.**
- Signed media grants, BYOS/managed/external origin delivery, playback attestations; until then video courses remain publish-blocked.
- Operational instructor/cohort enrollment, prerequisites, drip schedules and migration policies (SQL model exists; domain enforcement is not yet implemented).
- Certificate revocation with scoped step-up, audit and appeal; public verifier can read history but no protected revocation API yet.
- Minor/guardian policies, licensing jurisdiction, data export/retention, production monitoring/SLIs, real accessibility/device checks.
- Cross-surface API generated contract binding, reviewed idempotency/concurrency policies and production-like E2E with actual external provider, store policy, UI/browser and native devices.

**Do not mark this area 100%/VERIFIED/RELEASED merely because an API or unit test passes.** R8 is a post-launch capability and every required gate remains explicit until proven by CI and production-like evidence.

## Sequential external-commerce lifecycle proof (CI)

The PostgreSQL CI journey runs immediately after `ci_r2_mobile_store_invariants.sql` while its verified fixture entitlement is **ACTIVE**. This validates publication, paid enrollment, reading, human-graded assessment, completion, two store restarts and credential evidence using that existing captured payment chain. Afterward, the canonical R4 store refund/revocation invariant suite is applied, and a second Academy test explicitly proves that the **same student's** previously purchased raw lessons cannot be accessed or re-enrolled with a revoked entitlement. This deliberately tests authorization against the complete commerce lifecycle rather than artificially reactivating a refunded right.

## Author provenance and original copyrighted works

Publication is gated in PostgreSQL by an immutable, signed-session author declaration for the SHA-256 of the **exact current curriculum**, including modules, lesson content, grading rubrics and ordering. Editing the draft invalidates previous declarations, including from direct SQL. The author UI exposes acknowledgement and original provenance, and the API stores audit evidence and emits transactional outbox facts. This records a claimed original work; it is **not independent verification of authorship**, and content requiring third-party licensing cannot be admitted with this declaration. Video courses still cannot be published without real trusted playback attestation.

## Replay, forgery and retention proofs for author declarations

PostgreSQL integration now exercises repeated signed attestations (same immutable ID and provenance), no duplicate event, changed curriculum requiring a new snapshot, denial of foreign-author SQL insert, denial of UPDATE/DELETE against signed evidence, and refusal of new attestations after immutable publication. HTTP denial tests cover the new rights endpoint with missing signed sessions. External third-party licensing remains a separate authority; an original-work self-declaration must not be reinterpreted as a verified external contract.

## Canonical subscription revocation and refund

A store-verified transaction can leave its original `store_entitlements.state=ACTIVE` forever, while the later canonical R4 lifecycle projects `store_subscription_projections.entitlement_state=REVOKED` on a refund, chargeback or revocation. **Academy must consult both**, not just the original payment. `academy_entitlement_is_active(entitlement_id)` is now the single PostgreSQL access predicate used by enrollment, learning, progress, assessments, grading, completion, certificates and the signed-in author review queue. Subscription GRACE/HOLD events preserve access only when the canonical lifecycle projection's **entitlement** state remains `ACTIVE`. PostgreSQL regression proves a real R4 store refund removes access to protected lessons and disables re-enrollment, without falsifying the learner's historical completed course or already-issued certificate. Web and native surface expose a separate derived `access_state` and disable resumed learning after revocation.

## Author-scoped results and canonical gross capture evidence

The signed-author-only `GET /v1/academy/author/report` projects enrollment, completion,
entitlement access and reviewed assessment counts from Academy PostgreSQL.
The same endpoint exposes only **gross captured Store transactions** where
`store_transaction_verifications.product_ref` AND the immutable
`orders.product_id` match the Academy product, and the canonical payment
attempt, CAPTURED effect and ledger evidence all agree. No student identity,
answer text or payout account is returned. `revenue_state` is explicitly
`GROSS_CAPTURED_ONLY_NO_REFUND_COMMISSION_OR_PAYOUT_RECONCILIATION`.
Gross captures are **not** author income, net sales, royalties or payout.
The independent Finance owner must reconcile refunds, commissions and
settlements before any author-income claim is enabled.

Academy enrollment also checks the canonical order's product binding in
both Go and PostgreSQL: a store verification carrying a course-like product
reference but attached to an unrelated order cannot unlock a course.


## Learner assessment outcome readback (R8 follow-through)

The Academy PostgreSQL owner now projects bounded attempt history to the enrollment's
signed Identity. It returns attempt IDs, lesson IDs, submission timestamps, human-reviewed
results and scores; it deliberately does not expose raw responses or private grading
evidence. The Web Academy learner view displays PENDING/PASSED/FAILED and suppresses
retake controls while an attempt is pending or passed; the canonical PostgreSQL writer
remains the final authority, including for concurrent submissions. PostgreSQL integration
covers learner/foreign-owner isolation, pending and passed outcomes, duplicate denial,
and persistence after runtime restart. This readback is **not** independent exam proctoring,
trusted media playback, live PSP checkout or a production R8 release gate.

## Durable learner certificate readback

The signed learner's `GET /v1/academy/me/certificates` projects the existing
Academy certificate and immutable course-version title after reload, process
restart and Commerce refund. The credential state and the current **learning**
access state are separate: a refunded enrollment loses lessons, but does not
erase an earned certificate. The opaque public verification URL is visible only
to the learner through a `private, no-store` response. No raw answers,
identity of other students, or professional qualification is exposed. Web and
native learner surfaces expose historical certificates without repeating the
completion mutation. The PostgreSQL paid-journey and refund tests assert
restart persistence, foreign-identity isolation and revoked lesson access.

## R8 ↔ R4 Finance: no consultation settlement for Academy orders

The canonical R4 settlement/payout worker is consultation-only: it requires a
completed consultation session and a consultation commission policy. Some
Academy Store purchase fixtures still carry a legacy `orders.booking_id`.
That Booking association is **not** consultation fulfillment or authority to
settle educational revenue. Finance now excludes any order whose immutable
`orders.product_id` belongs to `academy_courses` from consultation settlement
candidate discovery, rejects direct `SettleOrder` attempts (including replay)
before reading an existing instruction, and repeats the exclusion when loading
consultation settlement facts. The isolated paid Academy PostgreSQL journey
asserts that a verified course purchase cannot produce a consultation
settlement instruction; existing R4 consultation convergence tests remain
required.

This is a **fail-closed safety boundary**, not a course revenue settlement
implementation. No Academy payout, provider execution or net author-income
claim is enabled. A future governed R8 Finance integration must supply
course-specific fulfillment, rights/royalty and refund evidence, a certified
provider capability, provider statements and reconciliation before a true
revenue figure can be displayed. Until then Academy exposes only captured
gross with explicit `GROSS_CAPTURED_ONLY_NO_REFUND_COMMISSION_OR_PAYOUT_RECONCILIATION`.

## PostgreSQL-enforced Finance settlement boundary (R8/R4)

Migration `000067_r8_finance_academy_sql_boundary.sql` adds two fail-closed
database guards around the **existing** consultation-only Finance settlement
instructions. A direct SQL insert for an Academy product is rejected even if
a caller bypasses `SettleOrder`; the reverse operation (registering a course
against a product with an existing consultation settlement) is rejected too.
Both paths lock the same canonical Product row before checking the other
table, preventing a concurrent check/insert race. The migration refuses to
install over already-contradictory historical settlement facts.

This is defense in depth for the R4 consultation boundary, **not** an Academy
Finance owner, net author-income reconciliation, or authorization to pay.
Existing course sales still require external PSP capture, refund/chargeback
and provider-statement evidence before reconciled author revenue is possible.
