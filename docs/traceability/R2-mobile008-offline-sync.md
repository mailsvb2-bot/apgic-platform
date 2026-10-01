# R2 evidence — APGIC-MOBILE-008 durable offline mutation sync

Requirement: `APGIC-MOBILE-008` — offline/retry must not create a duplicate Booking/Payment/Entitlement, and queued client state must remain distinct from server-confirmed state.

## Canonical runtime boundary

The implementation keeps the server authoritative and makes retry/restart safe without creating a second business truth on the device:

- the mobile checkout command invokes the same canonical `demand.Service.CreateCheckout` path used by the web/server flow;
- signed session Identity is derived server-side; the client cannot choose another Identity for the mutation record;
- the server derives a stable request digest from canonical checkout input and scopes the durable mutation by Identity, operation, idempotency key and immutable correlation id;
- PostgreSQL mutation records persist `CLAIMED`, terminal `APPLIED`, or terminal `FAILED`; replay of an applied mutation returns the existing side-effect reference, while a changed payload/correlation scope under the same idempotency key fails closed as conflict;
- concurrent claims use the canonical PostgreSQL idempotency boundary, so only one durable mutation/business side-effect can win;
- the installed client persists only a bounded INTERNAL mutation envelope (`hold_id`, `method_code`, idempotency/correlation identifiers, retry/expiry metadata and explicit local state), never canonical Booking/Payment truth;
- local states `LOCAL_PENDING`, `SYNCING`, `SERVER_CONFIRMED`, `CONFLICT`, and `FAILED` remain explicitly distinct;
- retry count and expiry are bounded; network/server unavailability returns the mutation to visible pending instead of falsely acknowledging it;
- Android SharedPreferences and iOS NSUserDefaults are allowed only through the audited offline-mutation storage adapters, with an 8192-byte bound and sensitive-marker rejection;
- after server confirmation, the durable native queue is cleared.

## Restart / lost-response proof

The native E2E server intentionally executes the first mobile checkout command successfully, commits the canonical side effect, then replaces the first successful response with a retryable 503. The installed app therefore remains `LOCAL_PENDING`.

The harness then force-stops/terminates the installed application and relaunches it without re-supplying the checkout payload. The application reloads the persisted queue, retries with the same idempotency/correlation scope, and resolves to the already-applied canonical checkout. A direct server replay is compared with the side-effect reference displayed by the installed application.

This proves the required boundary: lost acknowledgement plus app restart does not create a second checkout/booking side effect.

## Automated proof

Candidate `7c754e9ce67675560d82b47235e5924f80f58976` passed CI run `36917984621` (CI #1904).

Required evidence:

- **IDEMPOTENCY_TEST / SERVER** — PostgreSQL job `110557048448` passed `TestClientMutationPersistsAppliedFailedAndConflictOutcomes`, covering durable applied replay, terminal failed replay, changed-payload conflict and immutable mutation outcome.
- **CONCURRENCY_TEST / SERVER** — the same PostgreSQL job executed `backend/ci/r2_mutation_concurrency.sh` inside its migration/invariant proof and passed the concurrent-claim invariant suite.
- **NATIVE_E2E / ANDROID** — job `110557048250` built the APK and passed installed-app restart/retry E2E, including the lost committed response, force-stop, durable queue reload, second attempt and exact canonical side-effect replay.
- **NATIVE_E2E / IOS** — job `110557048282` built the simulator application and passed the equivalent terminate/relaunch durable queue recovery flow.
- **WEB / cross-surface regression** — job `110557048181` passed the Next.js production build and Playwright E2E.
- **Go race / HTTP contract proof** — job `110557047954` passed format, vet and race tests, including mobile checkout idempotency/correlation behavior.
- **Native TypeScript / queue-state proof** — job `110557048236` passed typecheck and unit tests for pending, retry, expiry, conflict and storage validation.
- **Canon / privacy / storage architecture** — job `110557048370` passed the full Canon/architecture battery, including native storage audit and generated-contract checks.
- **Restore/migration safety** — job `110557048227` passed the isolated PostgreSQL restore drill.
- **Multi-surface contract** — job `110557048405` passed.
- **Final bootstrap gate** — job `110564262245` passed.

Exact multi-surface evidence bindings:

- Web: `surface://WEB/mobile008-offline-sync/run-36917984621/job-110557048181`
- iOS: `surface://IOS/mobile008-offline-sync/run-36917984621/job-110557048282`
- Android: `surface://ANDROID/mobile008-offline-sync/run-36917984621/job-110557048250`
- Server/PostgreSQL: `surface://SERVER/mobile008-offline-sync/run-36917984621/job-110557048448`

Run artifacts bind the candidate to:

- release evidence artifact `11190759175`, digest `sha256:f02cb296186dd4cbb777f1d9e5d8186bf24c8ddd1a77cdde5653fb94e713cf67`;
- Android native artifact `11190168841`, digest `sha256:c16a9b5362b45b8b3d7b716b9a383206ab1cb1ed7041f51085bb5e191a994a6d`;
- iOS simulator artifact `11192115237`, digest `sha256:53d75b6df27d2b984849d2dfab2a049f6be1000f2e0c8542dd2cee2769905c3e`;
- restore-drill artifact `11191220970`, digest `sha256:1e662781ce9f66820adf084ab13378bd70d067b2f833aadf4cc5c0b8374eb2b4`;
- mobile SBOM artifact `11191420439`, digest `sha256:7462e6ddd9d764854f6b9cf12a7b3485beb438c4ffa980473b4b58495524d65a`.

The Requirement Registry can therefore advance to `VERIFIED`. This does **not** claim `RELEASED`: production rollout, real store signing/provider operations and production monitoring remain separate release evidence.
