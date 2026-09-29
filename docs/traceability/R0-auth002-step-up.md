# R0 AUTH-002 traceability

Requirement: HIGH_RISK actions require fresh step-up authentication and audit evidence. Without a fresh step-up, the mutation must not execute and the API must return a deterministic step-up requirement.

## Contract

- `contracts/openapi/apgic-v1.yaml`
  - `PATCH /v1/products/{productID}/commercial-owner`
  - signed client session required
  - operation is classified `HIGH_RISK`
  - missing or stale step-up returns `428 AUTH_STEP_UP_REQUIRED`
- `contracts/jsonschema/authorization-v1.schema.json`
  - authorization decisions include `STEP_UP_REQUIRED`
  - input carries `step_up_at`

## Implementation

- `backend/internal/httpapi/step_up.go`
  - short-lived signed step-up assertion
  - assertion is bound to the signed Identity
  - separate HMAC domain from the ordinary client session
  - no public endpoint exists to self-issue step-up
- `backend/internal/httpapi/product_ownership.go`
  - authenticates the signed Identity
  - checks canonical product ownership before disclosure/mutation
  - invokes `authz.Evaluator` with `RiskHigh`
  - performs the mutation only after `ALLOW`
- `backend/internal/runtimepostgres/product_ownership.go`
  - reads and updates the existing canonical `products` table
  - does not introduce a parallel ownership model
- `backend/internal/authz/authz.go`
  - missing, future or stale step-up fails closed

## Evidence

- `backend/internal/authz/authz_test.go`
  - domain proof for HIGH_RISK freshness semantics
- `backend/internal/httpapi/product_ownership_integration_test.go`
  - missing step-up: 428, no mutation, audit reason `AUTH_STEP_UP_REQUIRED`
  - stale step-up: 428, no mutation, audit reason `AUTH_STEP_UP_REQUIRED`
  - fresh signed step-up: mutation succeeds and authorization audit is `AUTH_ALLOWED`
- `.github/workflows/ci.yml`
  - runs the PostgreSQL-backed DB/API proof

## Trust boundary

This slice implements the verifier/session side of step-up. It intentionally exposes no public "issue step-up" endpoint. A future authentication-factor provider may issue the signed assertion only after completing its own stronger authentication flow.
