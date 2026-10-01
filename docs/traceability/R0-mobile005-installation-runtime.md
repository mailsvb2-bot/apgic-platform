# R0 evidence — APGIC-MOBILE-005 durable installation runtime

Requirement: `APGIC-MOBILE-005` — device installation and push identity.

## Canonical runtime path

The production server now owns a durable `ClientInstallation` lifecycle that is separate from user `Identity`:

- `POST /v1/mobile/installations` registers an iOS/Android installation under the signed client-session Identity. The caller cannot provide or override `identity_id`.
- `PATCH /v1/mobile/installations/{installationID}/push-endpoint` rotates a token without replacing the Identity and increments `push_generation` only for an actual token change.
- `POST /v1/mobile/installations/{installationID}/revoke` clears the endpoint and makes the installation ineligible for future push delivery.
- `GET /v1/mobile/installations` preserves active and revoked installation history for the owning Identity.
- Reinstall with the same push endpoint under the same Identity revokes the superseded installation before activating the new installation.
- A push endpoint that is active under a different Identity fails closed with `MOBILE_PUSH_ENDPOINT_CONFLICT`.
- PostgreSQL advisory transaction locks serialize installation-ID and push-endpoint mutation keys, preventing first-write and token-claim races from degrading into nondeterministic unique-constraint failures.

The API is versioned in `contracts/openapi/apgic-v1.yaml` and generated into the shared TypeScript client contract. Production readiness now requires the `client_installations` table.

## Production-capable native client boundary

The installed-app E2E no longer owns a separate HTTP implementation. `apps/mobile/src/mobile-installation-client.ts` exposes ordinary production-capable register, rotate, revoke and list functions that use the same versioned API contract with cookie credentials. The debug-only E2E orchestrator only supplies deterministic synthetic provider tokens and composes those production functions; it does not bypass or reimplement the client protocol.

The push provider that supplies a real APNs/Android provider token is intentionally outside this R0 lifecycle contract and remains subject to the later notification transport/provider work. The lifecycle here proves that whatever provider token is supplied is installation-scoped, rotates independently of Identity and becomes ineligible after revoke.

## Automated proof

`TestMobileInstallationHTTPPreservesIdentityAcrossRotationReinstallAndRevoke` executes the authenticated HTTP API against PostgreSQL and proves:

1. first registration and exact idempotent replay;
2. token rotation invalidates the stale token/generation;
3. same-token rotation is idempotent;
4. iOS + Android installations share one Identity without duplicating account truth;
5. reinstall with the current token revokes the superseded installation while preserving history;
6. another Identity cannot mutate an installation by known UUID;
7. another Identity cannot claim an active push endpoint;
8. revoke clears push eligibility and repeated revoke is idempotent.

The proof runs in the `postgres-invariants` CI job with an isolated PostgreSQL service.

## Verification boundary

The missing `NATIVE_E2E` evidence is now closed by CI run `36852204179` (`CI #1791`) for implementation candidate `9337addd3bec3399eede0455bf44ffc1d6064f11`.

- Android native job `110336244160`: debug APK build plus installed-app registration → push rotation → revoke lifecycle, conclusion `success`.
- iOS native job `110336244354`: simulator build plus the same installed-app lifecycle, conclusion `success`.
- PostgreSQL invariant job `110336244157`: authenticated HTTP lifecycle against canonical durable storage, conclusion `success`.
- Go format/vet/race, Canon/architecture conformance, Web production build, native typecheck, multi-surface contract guard and the final R0 bootstrap gate all concluded `success`.

Multi-surface evidence bindings:

- iOS: `surface://IOS/mobile-installation-lifecycle/run-36852204179/job-110336244354`
- Android: `surface://ANDROID/mobile-installation-lifecycle/run-36852204179/job-110336244160`

The installed applications use one signed client Identity while creating independent installation records, rotate the push endpoint without changing Identity, and finish with the canonical installation in `REVOKED` state at generation 2. Captured app accessibility state and server state are retained as workflow artifacts.

The Requirement Registry is therefore advanced to `VERIFIED`. This does **not** claim `RELEASED`: production signing, organization-owned store accounts and production rollout evidence remain separate release gates.
