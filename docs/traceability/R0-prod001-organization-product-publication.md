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
- `backend/migrations/000024_r0_published_product_order_gate.sql`
  - leaves historical Products and Orders unchanged as immutable business evidence;
  - makes PostgreSQL reject new orders unless the referenced product is PUBLISHED and the immutable ownership snapshot matches the canonical product.
- `backend/internal/runtimepostgres/organization_product.go`
  - requires ACTIVE Organization ownership before create/publish;
  - revalidates the direction inside the publication transaction so an archived direction returns the canonical conflict instead of surfacing as a database 500;
  - reads migration-compatible legacy DRAFT rows with nullable name/direction safely;
  - persists the Organization as explicit `owner_type=ORGANIZATION` and `owner_id`;
  - preserves explicit commercial owner, authors, revenue beneficiary and direction;
  - appends publication audit evidence atomically with the state transition.
- `backend/internal/runtimepostgres/journey.go`
  - materializes conformance catalog Products as PUBLISHED before checkout;
  - reconciles only an exact legacy canonical conformance DRAFT to PUBLISHED when its owner/roles/direction still match and the direction remains ACTIVE; divergent or archived truth fails closed;
  - never creates Product/Organization/Direction business truth inside payment checkout;
  - requires ACTIVE ownership/direction, PUBLISHED product state, exact ownership snapshot and slot/product binding before any order/payment side effect.
- `backend/internal/httpapi/organization_product.go`
  - exposes the signed-session runtime path.

## Web user flow

- `apps/web/app/organization/workspace.tsx`
  - loads products for the selected Organization;
  - creates a DRAFT only through the canonical Organization product endpoint;
  - requires the user to provide commercial owner, authors and revenue beneficiary explicitly;
  - publishes through the canonical publish endpoint;
  - reloads persisted PUBLISHED state after refresh.
- `apps/web/e2e/foundation.spec.ts`
  - proves create -> publish -> refresh through browser-visible controls and the canonical API contract.

## Automated evidence

- `backend/internal/httpapi/organization_product_integration_test.go`
  - creates Organization and direction;
  - creates a DRAFT product with explicit legal/revenue role refs;
  - publishes it;
  - proves PostgreSQL contains owner/commercial owner/authors/revenue beneficiary/direction/status;
  - proves publication audit evidence exists;
  - proves the published product is returned by the Organization product listing;
  - proves legacy nullable DRAFT rows remain readable;
  - proves archiving a direction before publish fails closed with `PRODUCT_DIRECTION_INVALID` and no publication audit side effect.
- `backend/internal/runtimepostgres/journey_test.go`
  - proves durable checkout rejects a DRAFT product;
  - proves the rejected checkout creates neither an order nor payment attempt;
  - preserves the restart/concurrency proof for a canonical pre-published product.
- `apps/web/e2e/foundation.spec.ts`
  - proves the product lifecycle is usable from the Organization Web workspace;
  - proves explicit legal/revenue role refs remain visible before and after publication;
  - proves published state survives a browser reload through the list endpoint.
- `.github/workflows/ci.yml`
  - applies migrations 000023 and 000024 in the migration proof and isolated restore drill;
  - runs the APGIC-PROD-001 publication, archived-direction, legacy-read and draft-checkout negative proofs.

The requirement remains IN_PROGRESS until fresh staging/release evidence proves the path outside CI. Code and contract completion alone are not represented as production verification.


## Deployed staging proof

Candidate `0df3a19bbcc67208b5f67ab07c57b40fa28c4470` is deployed on staging and the public canonical HTTP path was exercised outside CI. A Product was created under a generic Organization direction with explicit owner/commercial-owner/author/revenue-beneficiary refs, published, then re-read after the direction was archived. The Product remained `PUBLISHED` and all explicit ownership/revenue refs were preserved.

Evidence: `canon/evidence/staging-org-product-20261007T125100Z.json`.

The earlier “fresh staging/release evidence” gap is therefore closed. Registry promotion is deliberately separate because the global multisurface guard currently requires WEB/IOS/ANDROID evidence or approved applicability exceptions for launch requirements.
