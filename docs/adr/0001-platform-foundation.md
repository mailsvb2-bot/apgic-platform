# ADR-0001 — APGIC platform foundation

Status: Accepted for bootstrap implementation.

## Decision

APGIC is the owner of its canonical business truth. The backend is Go; durable business storage is PostgreSQL; Redis may only be introduced for cache/coordination. Web and native clients consume versioned contracts and do not own parallel booking, payment, qualification or ledger truth.

External systems are reached only through provider-neutral connector capabilities. ClientPlatform, BusinessAIOS, UCR and Virtual Persona Runtime are donor/reference or external provider systems, never APGIC runtime source-tree dependencies or shared-database owners.

## Bootstrap consequences

- one Go module for server domain boundaries;
- PostgreSQL migrations encode durable invariants;
- append-only audit and ledger evidence;
- transactional outbox boundary for external side effects;
- explicit tenant authorization and step-up decisions;
- separate PaymentProvider / PaymentMethod / PaymentRail semantics;
- contract-only shared client package;
- web and native surface directories exist from R0;
- machine-readable Canon Registry and Coverage Map are CI inputs.

This ADR does not mark R0 requirements VERIFIED. Evidence/status transitions remain governed by the Requirement Registry and release gates.
