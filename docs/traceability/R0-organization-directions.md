# R0 Organization direction traceability

## APGIC-ORG-001 — Organization universal boundary

- `backend/migrations/000020_r0_organization_direction_runtime.sql`
  - adds explicit `organization_ownerships` separate from membership;
  - adds generic non-empty `direction_type` without education-specific schema.
- `backend/migrations/000003_r0_semantic_invariants.sql`
  - remains the canonical source for the `organization_directions_no_delete` hard-delete guard.
- `backend/internal/runtimepostgres/organization_runtime.go`
  - creates Organization + ACTIVE membership + ACTIVE ownership atomically;
  - permits direction creation only to an ACTIVE owner.
- `backend/internal/httpapi/organization_runtime.go`
  - exposes organization creation/read and direction creation through the signed Identity session.
- `contracts/jsonschema/organization-v1.schema.json`
  - direction type is part of the canonical direction contract.
- `contracts/openapi/apgic-v1.yaml`
  - versioned public API contract for the runtime path.

## APGIC-ORG-002 — Deletion preserves dependent truth

- direction removal is represented by `status=ARCHIVED` and `archived_at`;
- no public hard-delete route exists;
- PostgreSQL trigger `organization_directions_no_delete` from `backend/migrations/000003_r0_semantic_invariants.sql` rejects direct DELETE attempts;
- `backend/internal/httpapi/organization_runtime_integration_test.go` proves:
  - organization ownership and membership persist separately;
  - a generic direction is created under its Organization;
  - archive preserves the direction row;
  - direct SQL DELETE fails;
  - the archived canonical row remains readable after the failed delete.

Historical linkage is now represented and proven beyond the Organization row itself:

- `docs/traceability/R0-org002-history-links.md` proves Product, Order, Booking, Ledger, Audit and store entitlement history survive archival and hard delete remains blocked;
- `canon/evidence/staging-org-product-20261007T125100Z.json` proves the deployed staging archive/readback path;
- installed iOS and Android app jobs in run `37628404273` prove the same canonical Organization/Product archive path on both native surfaces.

APGIC-ORG-002 is therefore `VERIFIED`.
