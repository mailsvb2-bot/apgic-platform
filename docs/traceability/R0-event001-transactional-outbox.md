# R0 EVENT-001 transactional outbox recovery proof

Requirement: domain state and its external side-effect event must commit atomically into PostgreSQL. A process failure after commit and before delivery must not lose the event; retry/restart must recover the same durable envelope and idempotency key without duplicating the domain effect.

## Implementation

- `backend/internal/runtimepostgres/outbox.go`
  - persists the outbox envelope inside the caller's PostgreSQL transaction;
  - protects semantic idempotency by `idempotency_key`;
  - detects collisions where the same idempotency key refers to different event content;
  - delivers only `PENDING` records;
  - uses a PostgreSQL advisory lock to prevent concurrent workers from delivering the same pending record at the same time;
  - increments attempts before provider delivery and moves to terminal `DELIVERED` only after acknowledgement.
- `backend/migrations/000004_r0_outbox_terminality.sql`
  - makes event identity/payload immutable;
  - prevents attempt counters from decreasing;
  - makes delivered rows terminal and immutable.
- `backend/internal/runtimepostgres/check.go`
  - booking/ledger commits use `ensureOutboxEventTx` in the same transaction as the canonical business mutation.

## PostgreSQL recovery proof

`backend/internal/runtimepostgres/outbox_integration_test.go` proves the failure/recovery sequence against real PostgreSQL:

1. starts a transaction containing a durable domain marker plus outbox event;
2. rolls the transaction back and proves neither side survives;
3. repeats and commits, proving both domain state and outbox exist together;
4. closes/reopens the PostgreSQL runtime to simulate process loss after commit but before delivery;
5. proves the pending envelope and its idempotency key survive restart;
6. simulates provider failure and proves the event remains pending with an incremented attempt counter;
7. restarts again, successfully delivers the same durable event, and marks it terminal;
8. runs delivery again and proves the delivered event is not emitted a second time.

The provider-facing envelope preserves a stable idempotency key across retries. External providers/connectors remain responsible for applying that key at their own execution boundary, while APGIC prevents duplicate canonical domain effects and duplicate terminal outbox emission.

## CI

`.github/workflows/ci.yml` runs `TestTransactionalOutboxSurvivesRollbackRestartAndRetry` inside the PostgreSQL migration/invariant job after the canonical migration chain has been applied.

This closes the repository-side durable recovery proof. APGIC-EVENT-001 remains `IN_PROGRESS` until release/staging evidence is attached according to the Canon evidence rules.
