# R0 CONN-001 — provider-neutral core continuity

Requirement: APGIC domain business truth must depend on capability boundaries rather than a concrete provider. If a CommunicationProvider is unavailable, canonical Identity, Booking and Ledger truth must remain owned and executable by APGIC without importing provider state into the core.

## Runtime boundary

- `backend/internal/connector/registry.go` defines provider-neutral capability classes and connector lifecycle states.
- `backend/internal/connector/executor.go` rejects unavailable/degraded/provider-mismatched execution at the connector boundary before invoking a provider.
- `backend/internal/runtimepostgres/journey.go` owns the durable core booking/checkout/payment-evidence path in PostgreSQL independently from communication-provider runtime state.

## PostgreSQL integration proof

`backend/internal/runtimepostgres/connector_boundary_integration_test.go`:

1. persists a `COMMUNICATION_PROVIDER` connector instance in `DISABLED` state;
2. creates one canonical client Identity and HelpIntent;
3. confirms intent, discovers a slot and acquires a durable booking hold;
4. appends an independent non-custodial Ledger evidence entry through the canonical PostgreSQL ledger store;
5. proves the Booking remains durably `HELD` without requiring communication-provider execution;
6. proves exactly one canonical Identity row and one Ledger effect exist;
7. proves the communication connector remains `DISABLED` throughout.

The test does not fabricate successful communication delivery and deliberately avoids payment-routing fixtures, so it is order-independent within the shared PostgreSQL CI database. It proves Identity, Booking and Ledger business truth do not become communication-provider-owned or fail merely because that provider is unavailable.

## CI evidence

The PostgreSQL invariant job runs `TestCoreJourneyContinuesWithCommunicationProviderUnavailable` against the canonical migration chain.

APGIC-CONN-001 remains `IN_PROGRESS`: this is repository/CI evidence for the required provider-neutral core invariant, not a production provider failover certification.
