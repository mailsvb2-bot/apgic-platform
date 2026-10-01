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

This closes the durable server/runtime portion and strengthens the `DOMAIN_OR_CONTRACT_TEST` evidence class. The Requirement Registry remains `IN_PROGRESS` because the Canon also requires `NATIVE_E2E`: an installed iOS/Android app must still demonstrate registration/rotation/revoke against this canonical server path before `APGIC-MOBILE-005` can be advanced to `VERIFIED`.

No production/store release claim is made here.
