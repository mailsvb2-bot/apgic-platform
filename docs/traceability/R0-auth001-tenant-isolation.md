# R0 AUTH-001 traceability

Requirement: protected actions must authorize principal/resource/scope/tenant, caller-supplied identifiers must not bypass tenant isolation, cross-tenant access must deny without data disclosure and persist audit evidence.

## Contract

- `contracts/openapi/apgic-v1.yaml`
  - `GET /v1/organizations/{organizationID}/private-profile`
  - signed client session required
  - `X-Organization-Context` is only a requested tenant context and is validated against durable ACTIVE membership
  - cross-tenant denial returns a deterministic authorization reason and no private organization payload

## Implementation

- `backend/internal/httpapi/organization_authz.go`
  - resolves signed Identity from the trusted session
  - validates requested organization context against durable membership
  - invokes `authz.Evaluator` before private resource access
  - reads private organization data only after `ALLOW`
- `backend/internal/runtimepostgres/organization_access.go`
  - durable ACTIVE membership lookup
  - same-tenant private organization read
- `backend/internal/authz/evaluator.go`
  - principal/resource tenant comparison
  - append-only authorization audit evidence
  - fail-closed behavior when audit persistence is unavailable

## Required evidence

- SECURITY_TEST:
  - `backend/internal/authz/evaluator_test.go`
- DB/API_NEGATIVE_TEST:
  - `backend/internal/httpapi/organization_authz_integration_test.go`
  - Organization A member requests Organization B private resource
  - response is `403 AUTH_CROSS_TENANT_DENY`
  - Organization B private name is absent from the response
  - PostgreSQL `audit_records` contains `decision=DENY`, `AUTH_CROSS_TENANT_DENY`, the principal scope, resource tenant and correlation id
  - forged `X-Organization-Context` for Organization B is rejected because membership is not derived from caller input

## CI gate

`.github/workflows/ci.yml` runs `TestCrossTenantOrganizationHTTPDeniesWithoutDisclosureAndAudits` against the real PostgreSQL schema.


## WEB surface proof

`apps/web/e2e/auth001-postgres.spec.ts` is a dedicated PostgreSQL-backed browser proof. It is skipped by the ordinary conformance-only web suite and enabled only by the AUTH-001 database gate.

The proof uses two independent browser contexts:

1. Browser/session A creates Organization A through `POST /v1/organizations`.
2. Browser/session B creates Organization B through the same public API.
3. Browser A reads its own private profile with `X-Organization-Context: A` and receives ALLOW.
4. Browser A requests Organization B while keeping tenant context A and receives `403 AUTH_CROSS_TENANT_DENY` without the private B name.
5. Browser A forges `X-Organization-Context: B` and receives `403 AUTH_TENANT_CONTEXT_DENIED`, again without private B data.
6. The CI job queries PostgreSQL `audit_records` for both deterministic correlation IDs and requires persisted DENY evidence.

The browser path is `Browser -> Next /v1 proxy -> APGIC API -> PostgreSQL`; it does not mock the authorization decision or the database.

AUTH-001 remains `IN_PROGRESS` until this fresh WEB proof and explicit IOS/ANDROID surface evidence are bound to an exact green candidate.


## Native installed-app surface proof

AUTH-001 is exercised through the installed React Native app on both native surfaces, not by a JavaScript-only mock:

- `apps/mobile/src/mobile-authz-e2e.ts` calls the canonical `GET /v1/organizations/{organizationID}/private-profile` endpoint with the signed client session and explicit organization context.
- `backend/cmd/mobile-installation-e2e-server/main.go` wires the production `httpapi` authorization handler and `authz.Evaluator` to an E2E-only organization store. The handler persists the same append-only audit records used by the canonical authorization path.
- The native client proves same-tenant ALLOW, `AUTH_CROSS_TENANT_DENY`, forged-context `AUTH_TENANT_CONTEXT_DENIED`, absence of a sentinel foreign private value in denial payloads, and persisted DENY audit evidence.
- `tools/mobile_android_capability_e2e.sh` launches the installed Android APK with the AUTH-001 inputs and requires all seven proof labels.
- `tools/mobile_ios_capability_e2e.sh` launches the installed iOS simulator app with the same proof contract and requires the same labels through AXBridge.
- The proof artifacts are emitted as `evidence/android-capability-e2e-auth001.xml` and `evidence/ios-capability-e2e-auth001.json`, so the existing native artifact upload retains them with the exact CI candidate.

## Exact verification evidence

Candidate `cfcfb889039f13e4c96d9657f0b3916a20a57211` passed CI run `37529803218` with the complete cross-surface proof chain:

- WEB + PostgreSQL tenant-isolation proof: job `112496666549`; artifact `auth001-web-tenant-isolation-evidence` (`11444285351`, SHA-256 `a935c5f330aaae618395c7fe21f738ebd774c46b46094fe26ca28e2c0bafd6b4`).
- Android installed-app proof: job `112496667106`; artifact `android-native-debug-build` (`11444890605`, SHA-256 `9d8ccf735195039e9e04cc246e5909f16063df685c727c7c9ac0b76fe8f1aab0`).
- iOS installed-app proof: job `112496666841`; artifact `ios-native-simulator-build` (`11444986832`, SHA-256 `1b31a85865dfe61dcbea4cd09ff95d96e31ab8f41548bc731cdd7a397c61b982`).
- Go domain/runtime tests: job `112496667053`.
- Canon / architecture conformance: job `112496666852`.
- Final R0 bootstrap gate: job `112504321488`.

The Android and iOS artifacts are bound to the exact candidate SHA and contain the installed-app AUTH-001 evidence files emitted by the native harness. The status/evidence-binding commit changes governance metadata only; it does not change the proved authorization implementation.

## Release status

APGIC-AUTH-001 is **VERIFIED**. This verifies canonical tenant isolation and audit behavior across WEB/iOS/Android/server evidence surfaces; it does not claim a production deployment. Any subsequent implementation/contract/test change affecting AUTH-001 requires fresh verification evidence before release promotion.
