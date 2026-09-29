# R0 CONN-002 atomic webhook effect proof

Requirement: a repeated or out-of-order provider webhook must never cause a duplicate booking/economic/domain effect. Receipt state may only become APPLIED when the corresponding canonical domain mutation commits in the same PostgreSQL transaction.

## Defect closed

The previous delivery state machine could mark a receipt APPLIED, or promote a deferred receipt to APPLIED, without executing the corresponding business effect. Deferred receipts also did not preserve the original payload, so an out-of-order event could not be durably replayed after the sequence gap closed.

## Durable state machine

- `backend/migrations/000022_r0_connector_atomic_application.sql`
  - stores the verified webhook payload on the immutable delivery receipt;
  - introduces READY as the state between receipt acceptance and committed domain effect;
  - registration returns APPLY only after locking the canonical stream position, but does not advance the stream;
  - `apgic_finalize_connector_delivery` marks APPLIED and advances the stream only after the caller has executed the business mutation in the same transaction;
  - deferred promotion changes DEFERRED → READY only; it never claims the effect was applied;
  - duplicate/stale evidence remains immutable and stream positions cannot rewind.

## Runtime

- `backend/internal/runtimepostgres/connector_delivery.go`
  - verifies the Ed25519 webhook signature before persistence;
  - canonicalizes and hashes the payload;
  - registers the receipt and executes the domain callback inside one PostgreSQL transaction;
  - finalizes the receipt and stream position in that same transaction;
  - replays deferred payloads from PostgreSQL and rolls READY promotion back to DEFERRED if the business callback fails.

## Evidence

- `backend/internal/runtimepostgres/connector_delivery_integration_test.go`
  - sequence 2 arrives before 1 and is durably deferred;
  - sequence 1 applies exactly once;
  - duplicate sequence 1 does not call the domain effect again;
  - deferred sequence 2 is replayed from its stored payload and applies once;
  - a failed immediate domain effect rolls back both receipt and stream movement;
  - a failed deferred domain effect rolls READY back to DEFERRED and leaves stream position unchanged;
  - retry succeeds once and converges the stream without duplicate effects.
- `backend/migrations/ci_r0_connector_delivery_invariants.sql`
  - proves READY is not APPLIED;
  - proves stream position does not move before finalization;
  - proves deferred payload survives and promotion does not falsely claim application.
- `.github/workflows/ci.yml`
  - applies migration 000022 in PostgreSQL and restore-drill chains;
  - runs the PostgreSQL-backed runtime proof.

APGIC-CONN-002 remains IN_PROGRESS until release/staging evidence is attached. Repository-side code and automated proof must not be represented as production verification.
