# R0 AUTH-002 traceability

Requirement: HIGH_RISK actions require fresh step-up authentication and audit evidence. Without a fresh step-up, the mutation must not execute and the API must return a deterministic step-up requirement.

## Contract

- `contracts/openapi/apgic-v1.yaml`
  - `PATCH /v1/products/{productID}/commercial-owner`
  - signed client session required
  - operation is classified `HIGH_RISK`
  - missing, stale or session-mismatched step-up returns `428 AUTH_STEP_UP_REQUIRED`
- `contracts/jsonschema/authorization-v1.schema.json`
  - authorization decisions include `STEP_UP_REQUIRED`
  - HIGH_RISK input can carry `session_ref`, `step_up_at` and `step_up_method`

## Implementation

- `backend/internal/httpapi/client_session.go`
  - newly issued sessions contain a unique signed session id
  - legacy v1 sessions remain readable for ordinary session continuity
  - audit/step-up code receives only a one-way `session:...` reference, never the raw cookie
- `backend/internal/httpapi/step_up.go`
  - short-lived signed step-up assertion
  - assertion is bound to Identity + exact signed client session + authentication method
  - separate HMAC domain from the ordinary client session
  - no public endpoint exists to self-issue step-up
- `backend/internal/httpapi/product_ownership.go`
  - authenticates the signed Identity
  - derives the current signed session reference
  - accepts step-up evidence only when it is bound to that same session
  - invokes `authz.Evaluator` with `RiskHigh`
  - performs the mutation only after `ALLOW`
- `backend/internal/authz/authz.go`
  - missing, incomplete, future or stale step-up fails closed
- `backend/internal/authz/evaluator.go`
  - every HIGH_RISK authorization audit snapshot records principal, session reference, method (when present), step-up timestamp, evaluation timestamp, decision, reason and policy version
- `backend/internal/runtimepostgres/product_ownership.go`
  - reads and updates the existing canonical `products` table
  - does not introduce a parallel ownership model

## Evidence

- `backend/internal/authz/authz_test.go`
  - domain proof for HIGH_RISK freshness and complete evidence semantics
- `backend/internal/authz/evaluator_test.go`
  - HIGH_RISK audit snapshot contains security evidence
- `backend/internal/httpapi/client_session_test.go`
  - separate sessions for one Identity receive distinct signed session values/references
- `backend/internal/httpapi/step_up_test.go`
  - step-up cannot cross Identity or session boundaries
  - expired/invalid assertions fail closed
- `backend/internal/httpapi/product_ownership_integration_test.go`
  - missing step-up: 428, no mutation, audit reason `AUTH_STEP_UP_REQUIRED`
  - stale step-up: 428, no mutation, audit reason `AUTH_STEP_UP_REQUIRED`
  - step-up for a different session: 428, no mutation
  - fresh same-session signed step-up: mutation succeeds and authorization audit is `AUTH_ALLOWED`
  - PostgreSQL audit evidence preserves principal/session/method/timestamps/policy decision
- `.github/workflows/ci.yml`
  - runs the PostgreSQL-backed DB/API proof

## Remaining production dependency

This slice implements the verifier/session/evidence side of step-up and closes session replay within APGIC. It intentionally exposes no public "issue step-up" endpoint. A production IdentityProvider / WebAuthn / MFA factor flow must mint the assertion only after a real stronger-authentication ceremony. AUTH-002 therefore remains `IN_PROGRESS` until that externally backed end-to-end user journey has release evidence.
