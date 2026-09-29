# R0 Organization direction traceability

## APGIC-ORG-001 — Organization universal boundary

- `backend/migrations/000020_r0_organization_direction_runtime.sql`
  - adds explicit `organization_ownerships` separate from membership;
  - adds generic non-empty `direction_type` without education-specific schema.
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
- PostgreSQL trigger `organization_directions_no_delete` rejects direct DELETE attempts;
- `backend/internal/httpapi/organization_runtime_integration_test.go` proves:
  - organization ownership and membership persist separately;
  - a generic direction is created under its Organization;
  - archive preserves the direction row;
  - direct SQL DELETE fails;
  - the archived canonical row remains readable after the failed delete.

This slice does not claim historical booking/product linkage that is not present in the current schema. ORG-002 remains IN_PROGRESS until every downstream commercial/booking relation that refers to a direction is itself represented and proven to survive archival.
