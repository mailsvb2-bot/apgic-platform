# R0 APGIC-PROD-001 organization product publication proof

APGIC-PROD-001 requires a specialist to create a Product inside an Organization and, at publication time, have explicit owner, commercial owner, author and revenue-beneficiary roles available to immutable downstream order snapshots.

## Contract

- `contracts/openapi/apgic-v1.yaml`
  - creates and lists Organization-owned product drafts;
  - publishes a product through an explicit endpoint;
  - requires `commercial_owner_ref`, `author_refs` and `revenue_beneficiary_ref` in the create request instead of inferring them from `owner_id`.
- `contracts/jsonschema/product-ownership-v1.schema.json` remains the canonical ownership-role vocabulary.

## Durable implementation

- `backend/migrations/000023_r0_product_runtime.sql`
  - adds DRAFT/PUBLISHED lifecycle metadata;
  - keeps legacy products migration-compatible as DRAFT;
  - fails closed when a PUBLISHED Product lacks a name or explicit ownership/revenue roles;
  - requires Organization-owned published products to reference an ACTIVE direction owned by the same Organization.
- `backend/internal/runtimepostgres/organization_product.go`
  - requires ACTIVE Organization ownership before create/publish;
  - persists the Organization as explicit `owner_type=ORGANIZATION` and `owner_id`;
  - preserves explicit commercial owner, authors, revenue beneficiary and direction;
  - appends publication audit evidence atomically with the state transition.
- `backend/internal/httpapi/organization_product.go`
  - exposes the signed-session runtime path.

## Automated evidence

- `backend/internal/httpapi/organization_product_integration_test.go`
  - creates Organization and direction;
  - creates a DRAFT product with explicit legal/revenue role refs;
  - publishes it;
  - proves PostgreSQL contains owner/commercial owner/authors/revenue beneficiary/direction/status;
  - proves publication audit evidence exists;
  - proves the published product is returned by the Organization product listing.
- `.github/workflows/ci.yml`
  - applies migration 000023 in the migration proof and isolated restore drill;
  - runs the APGIC-PROD-001 integration proof.

The requirement remains IN_PROGRESS until fresh staging/release evidence proves the path outside CI. Code and contract completion alone are not represented as production verification.
