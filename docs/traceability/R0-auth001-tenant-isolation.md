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
