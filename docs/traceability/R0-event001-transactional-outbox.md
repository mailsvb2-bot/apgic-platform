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
- `backend/internal/eventspine/http_delivery.go`
  - serializes the canonical Event Envelope to a replaceable HTTPS Event Gateway;
  - preserves the durable idempotency key as both body data and the `Idempotency-Key` header;
  - sends only an explicit scoped service principal/credential boundary;
  - treats every non-2xx/downstream transport error as delivery failure, so the record cannot be marked `DELIVERED`.
- `backend/cmd/outbox-worker/main.go`
  - is a dedicated runtime process; delivery is not hidden inside request handling;
  - polls durable `PENDING` records and survives provider/gateway outage without losing the event.
- `deploy/staging/apgic-outbox-worker-staging.service`
  - provides a sandboxed systemd runtime for staging when explicitly enabled.
- `deploy/staging/check-staging-runtime.sh`
  - when the worker is enabled, requires the service to be active and fails the watchdog on a stale pending backlog.

## PostgreSQL recovery proof

`backend/internal/runtimepostgres/outbox_integration_test.go` proves the failure/recovery sequence against real PostgreSQL:

1. starts a transaction containing a durable domain marker plus outbox event;
2. rolls the transaction back and proves neither side survives;
3. repeats and commits, proving both domain state and outbox exist together;
4. closes/reopens the PostgreSQL runtime to simulate process loss after commit but before delivery;
5. proves the pending envelope and its idempotency key survive restart;
6. simulates provider failure and proves the event remains pending with an incremented attempt counter;
7. restarts again, successfully delivers the same durable event, and marks it terminal;
8. runs delivery again and proves the delivered event is not emitted a second time;
9. proves a failed event blocks later delivery only for the same aggregate, while an unrelated aggregate continues, preventing global head-of-line starvation without violating per-aggregate ordering.

The provider-facing envelope preserves a stable idempotency key across retries. External providers/connectors remain responsible for applying that key at their own execution boundary, while APGIC prevents duplicate canonical domain effects and duplicate terminal outbox emission. Payloads are now required to match the canonical JSON object contract instead of accepting arbitrary valid JSON.

## Runtime activation boundary

The worker is intentionally fail-closed and is not silently enabled against a fake sink. Staging activation requires:

- `APGIC_OUTBOX_WORKER_ENABLED=1`;
- HTTPS `APGIC_EVENT_GATEWAY_URL`;
- `APGIC_EVENT_GATEWAY_PRINCIPAL_ID`;
- a minimum 32-byte `APGIC_EVENT_GATEWAY_CREDENTIAL`;
- an Event Gateway that acknowledges only after accepting the idempotent canonical envelope.

Without those externally backed values the updater keeps the worker disabled and does not claim staging delivery evidence.

## CI

`.github/workflows/ci.yml` runs the outbox unit/race suite and PostgreSQL recovery/invariant proofs after the canonical migration chain has been applied. HTTP delivery tests prove exact envelope/idempotency/scoped-auth behavior and fail-closed non-2xx handling.

This closes the missing repository-side production worker path. APGIC-EVENT-001 remains `IN_PROGRESS` until the worker is enabled on the authorized APGIC staging host against a real compatible Event Gateway and a staging proof demonstrates commit -> process restart -> external acknowledgement -> terminal `DELIVERED` with no duplicate domain effect.
