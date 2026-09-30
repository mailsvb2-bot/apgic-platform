# R0 TERM-001 — provider-neutral terminology persistence proof

APGIC-TERM-001 requires canonical domain/API/storage terminology to use capability classes while provider/vendor brands remain implementation details behind connector boundaries.

## PostgreSQL proof

`backend/internal/runtimepostgres/provider_terminology_integration_test.go` exercises the canonical connector persistence model:

1. persists two different provider kinds behind the same `COMMUNICATION_PROVIDER` capability;
2. proves both rows retain vendor-specific `provider_kind` values while the canonical `capability_class` is identical;
3. proves generated `execute_scope` depends only on `capability_class`, not on provider/vendor kind;
4. attempts to persist `ClientPlatform` as a canonical capability class and requires the database constraint to reject it.

This is deliberately a database-backed proof, not only a source-code naming lint. It shows the runtime persistence contract keeps provider names behind the capability boundary.

## CI gate

The PostgreSQL invariant job runs `TestConnectorPersistenceKeepsProviderKindBehindCanonicalCapability` against the canonical migration chain.

APGIC-TERM-001 remains `IN_PROGRESS`: the repository proof closes the required E2E evidence class without claiming all future provider integrations are already certified.
