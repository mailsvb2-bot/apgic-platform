# R2 evidence — APGIC-MOBILE-007 notification transport parity

Requirement: `APGIC-MOBILE-007` — push is a transport of canonical `NotificationIntent` and must preserve shared preferences, idempotency, audit and sensitive-content rules.

## Canonical runtime boundary

The implementation keeps one notification business truth and prevents the mobile transport from becoming a second notification state machine:

- `NotificationIntent` remains the canonical business notification entity.
- PostgreSQL stores recipient Identity, channel preferences, policy version, consent basis and sensitive-preview policy on the canonical intent boundary.
- Provider delivery is claimed through a stable `intent + channel + endpoint` idempotency key.
- The server re-reads canonical channel preferences immediately before the provider-side delivery claim. A stale client/cache preference cannot suppress or enable provider delivery.
- Duplicate provider delivery requests return the already claimed delivery instead of creating a second business/provider delivery row.
- Every delivery decision, including a duplicate retry, writes immutable audit evidence under the same correlation/idempotency key.
- Transactional notification delivery cannot inherit MARKETING consent.
- Sensitive push transport uses a minimal opaque envelope containing only `contract_version`, `delivery_id` and `intent_id`; raw notification/consultation text is not carried in the transport payload.
- The installed client re-reads an owner-scoped canonical projection from the authenticated server endpoint and fails closed if the delivery/intent scope differs.
- Sensitive data classes require `GENERIC` preview on push unless canonical server policy explicitly permits full preview.

## Versioned cross-surface contract

The authenticated projection endpoint is published in `contracts/openapi/apgic-v1.yaml` and generated into the shared TypeScript API contract. The mobile transport/projection shapes are also represented in the R2 transaction-edge contract/schema.

Production readiness now requires the canonical `notification_intents` and `notification_deliveries` tables. Migration `000025_r2_notification_policy_runtime.sql` intentionally leaves transaction control to the staging migration runner so schema application and migration-ledger recording remain atomic.

## Automated proof

Candidate `d2e78dcdeb9315a3194daf9090e97c337537f441` passed CI run `36906994292` (CI #1873).

Required evidence:

- DOMAIN_OR_CONTRACT_TEST: Go format/vet/race job `110519955172` passed the notification transport domain tests.
- PostgreSQL runtime proof: job `110519955475` passed `TestNotificationTransportClaimIsIdempotentAuditedAndOwned`, proving one provider delivery row under retry, two immutable audit decisions, owner isolation, server-authoritative preferences and `GENERIC` sensitive preview.
- Contract generation / architecture: Canon job `110519955512`, native typecheck job `110519955528` and multi-surface contract job `110519955789` all passed.
- NATIVE_E2E / IOS: job `110519955325` passed the installed simulator application flow and resolved the canonical notification transport to the expected intent with `GENERIC` preview.
- NATIVE_E2E / ANDROID: job `110519955578` passed the installed emulator application flow and the same canonical notification transport projection.
- Web production build: job `110519954885` passed, showing the contract/runtime changes did not break the web surface.
- Restore/migration safety: job `110519955103` passed.
- Final bootstrap gate: job `110527335395` passed.

Exact multi-surface evidence bindings:

- iOS: `surface://IOS/mobile007-notification-transport/run-36906994292/job-110519955325`
- Android: `surface://ANDROID/mobile007-notification-transport/run-36906994292/job-110519955578`
- Server/PostgreSQL: `surface://SERVER/mobile007-notification-transport/run-36906994292/job-110519955475`

Run artifacts bind the candidate to:

- release evidence artifact `11184489937`, digest `sha256:6705a5cf31f1c9e2c8bbc56ea42cb89a53d9694636b254952850ddc60ac09e1d`;
- iOS simulator artifact `11185428522`, digest `sha256:fc894083721d079fa550d99a489838a8b4d6a74cd79c72f8baf7e1f4944aa7b8`;
- Android debug artifact `11184548045`, digest `sha256:24abee52e457e77202e7bc380e0305640725fa3d20f5ad399c7081550cf77b80`.

The Requirement Registry can therefore advance to `VERIFIED`. This does **not** claim `RELEASED`: real production APNs/FCM provider credentials, organization-owned signing/store artifacts and live production rollout remain separate release evidence.
