# R9 Live Events — isolated slice execution evidence

Source: Canon v7 §§ 116, 123, 184–185, 246, 249; registry release profile **R9** (depends on R3).
Governance gap: registry v7 does not currently contain dedicated APGIC-LIVE-* Requirement IDs. Do **not** promote to VERIFIED/RELEASED without Requirement Change/RFC and provider staging proof. R9 is post-launch; none of this branch is authorized for main or production yet.

## Ownership and invariants

- APGIC: live_events, registration, capacity, lifecycle, attendance evidence, outbox, organization tenant boundary.
- CommunicationProvider (replaceable): actual SFU, rooms, signaling, media tickets, gateway, attendance and recording workloads.
- NotificationProvider: delivery and reminders; payment provider: monetary execution, never APGIC.
- No direct donor import, shared production DB, client-provided attendance, stored media in business database or implicit provider SDK.
- Creation requires an active organization owner and a scoped session, plus an idempotency key. Registration is bound to a trusted session and serialized by the locked event row.
- Paid registration is **not** implemented; rejects price_minor > 0 rather than creating fictional funds/entitlements.
- Provider readiness uses signed connector delivery evidence, but **media ticket issuance is not yet wired**. Eligibility endpoint expressly does not return an access token. Recording=false, with no synthetic recording entitlement.
- Creation and registration side-effects are in the same transaction as transactional outbox persistence. Signed provider delivery receipts are applied atomically.
- Consent, minors, privacy, compliance, cancellation/refund, offer rights and jurisdiction gates require additional R9 specification before opening regulated or paid formats.

## API (feature gate APGIC_LIVE_EVENTS_ENABLED=1)

- GET /v1/live-events — public catalogue of published events
- GET /v1/organizations/{organizationID}/live-events — authenticated active owner catalogue, including saved drafts
- GET /v1/organizations/{organizationID}/live-events/{eventID}/attendance — owner-only signed provider attendance summary, no raw content
- GET /v1/live-events/{eventID} — public details, draft only for owners
- POST /v1/organizations/{organizationID}/live-events — owner, JSON draft, Idempotency-Key (16..160 chars)
- POST /v1/live-events/{eventID}/{publish|start|complete|cancel} — owner, verified provider proof for publish/start
- POST /v1/live-events/{eventID}/registrations — session-bound, free registration
- GET /v1/live-events/{eventID}/registration — current identity only
- POST /v1/live-events/{eventID}/unregister — current identity only
- GET /v1/live-events/{eventID}/access — access *eligibility*, fail closed if media tickets unavailable
- POST /v1/live-provider-webhooks — signed, sequence-checked provider CAPABILITIES/JOINED/LEFT evidence.

## Acceptance / release blockers

- [x] Domain state machine, price gate, join decision negative unit tests (code-level).
- [x] PostgreSQL schema: event/registration/immutable attendance and capacity-row lock (requires database integration verification).
- [x] HTTP route auth, unavailable provider / unsigned evidence, no fabricated media ticket tests.
- [x] Public web catalogue, event detail, free-registration UI and organization draft/publish/finish control panel (browser E2E pending).
- [x] Native iOS/Android event catalogue and registration surface behind explicit opt-in flag (native builds pending).
- [x] Private owner attendance aggregate from signed provider JOINED/LEFT facts (requires external attendance callback staging proof).
- [x] PostgreSQL migration and row-level invariants executed in CI, including cross-tenant denial, capacity and signed attendance projection.
- [ ] Signed room ticket issuing provider connector; actual WebRTC/SFU with verified health, capacity, fallback and restoration.
- [ ] Native iOS/Android production navigation enablement and end-to-end room delivery.
- [ ] NotificationPolicy versions and PT24H/PT2H/PT15M/AT_START reminders through existing notification delivery.
- [ ] Real paid Order/PSP/Entitlement/Refund/financial reconciliation before price_minor>0 can be enabled.
- [ ] Attendance provider integration e2e, post-event follow-up/Offer, privacy consent/recording egress, moderator controls and abuse response.
- [ ] SQL high-contention concurrency tests, real provider signed callbacks and cross-tenant API tests against a production-like Postgres staging cluster.
- [ ] Requirement Registry change and staging production-like evidence with SLO, security, accessibility and responsive checks.

No checkbox above means production completion without the corresponding external evidence. Keep feature **OFF** until completion.

## Code and CI evidence

- API handlers and Postgres-backed domain: `backend/internal/httpapi/live_events.go`, `backend/internal/runtimepostgres/live_events*.go`.
- SQL migration and isolated invariant checks: `backend/migrations/000032_r9_live_events.sql`, `backend/migrations/ci_r9_live_events_invariants.sql` (wired into CI).
- Web: `apps/web/app/live-events/`, `apps/web/app/organization/live-events/`; native: `apps/mobile/src/r9-live-events.tsx`.
- Run `go test -race ./...` and GitHub Actions CI for exact final branch SHA; green job on an earlier SHA is not release evidence for later commits.
- Provider room/ticket/scheduled reminders/payment/recording are explicit release blockers and must not be marked complete from mocks or signed capability heartbeat alone.

## Lifecycle and deployment hardening

- Re-registering after cancellation atomically reclaims a released seat when the published event has capacity; historical outbox facts remain immutable.
- Public and participant event reads never expose organizer identity UUIDs.
- Enabling `APGIC_LIVE_EVENTS_ENABLED=1` without canonical PostgreSQL is a startup error; server readiness checks all R9 tables and SQL guard procedures. Feature disabled is the default.
- Playwright smoke coverage checks truthful unavailable state, organizer route, and unchanged critical-entry baselines.
- Signed provider heartbeats alone **never** turn the media eligibility response into a room token. Actual access still needs the separately certified communication provider.

## SQL regression root cause and prevention (2026-10-12)

The first R9 SQL acceptance script was silently damaged by string substitution of PostgreSQL `$$` quoting into `$`. This was not a PostgreSQL deployment failure: GitHub Actions reached the SQL script and PostgreSQL rejected `DO $`.

**Permanent verification boundary**:
- `tools/r9_sql_contract_guard.py` refuses unbalanced or truncated SQL dollar quoting, missing negative controls, disabled CI concurrency steps; its regression tests are part of Canon CI.
- `ci_r9_live_events_invariants.sql` is a rollback-only SQL acceptance suite. It asserts **the exact expected denial reason**, not merely any exception, for foreign owners, draft and unpaid registration, provider proof, lifecycle, scope, capacity, cancellation and re-entry, duplicate signed attendance, immutable evidence and terminal states.
- `backend/ci/r9_registration_concurrency.sh` uses 24 independent psql clients contending for three real PostgreSQL seats, checks exactly three admissions, all remaining denials and no duplicate participant seats.
- The GitHub Actions PostgreSQL job runs the migration, the rollback-only suite, and concurrent registration proof against the same database. The R0 bootstrap gate stays dependent on genuine PostgreSQL success; it must not be bypassed.
- The proof suite simulates a stored verified provider reference for DB invariants. It is **not proof of live media**, actual signed UCR grants, production readiness or financial settlement.

CI green is necessary but not sufficient for R9 release. A production-like signed media join, attendance provider outcomes and consent/refund journeys remain release blockers.
