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
- [x] Public web catalogue, event detail, free-registration UI and organization draft form (pending browser E2E).
- [ ] Signed room ticket issuing provider connector; actual WebRTC/SFU with verified health, capacity, fallback and restoration.
- [ ] Native iOS/Android navigation and end-to-end room delivery.
- [ ] NotificationPolicy versions and PT24H/PT2H/PT15M/AT_START reminders through existing notification delivery.
- [ ] Real paid Order/PSP/Entitlement/Refund/financial reconciliation before price_minor>0 can be enabled.
- [ ] Attendance provider integration e2e, post-event follow-up/Offer, privacy consent/recording egress, moderator controls and abuse response.
- [ ] SQL concurrency and negative tenant tests against an actual Postgres cluster.
- [ ] Requirement Registry change and staging production-like evidence with SLO, security, accessibility and responsive checks.

No checkbox above means production completion without the corresponding external evidence. Keep feature **OFF** until completion.
