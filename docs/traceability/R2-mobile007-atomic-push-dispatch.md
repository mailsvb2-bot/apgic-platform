# R2 / APGIC-MOBILE-007 — atomic, durable push dispatch reservation

## User-facing failure

A canonical notification delivery may be claimed repeatedly with the same idempotency key.
That correctly avoids a second `notification_deliveries` row, but does not grant exclusive
permission to call an external push provider: two workers could use the same stale PENDING
plan and send the user duplicate notifications.

## Canonical owner and scope

- The durable dispatch owner is the APGIC PostgreSQL `notification_push_dispatches` table.
  Each `delivery_id` has one immutable dispatch reservation.
- `PushDispatchService.Reserve` only returns the minimal opaque push envelope after the
  canonical store has committed the reservation and its immutable audit record.
- `ClaimNotificationDelivery` returns the notification's business identity and status,
  **not** permission to send. The previous direct public `PushEnvelope` factory is removed.
- The insert checks delivery ID, intent, provider, endpoint, booking, policy version,
  sensitive data class, state PENDING, canonical PUSH preference and server-clock time window.
  The unique primary key linearizes competing workers; stale PENDING snapshots do not bypass it.
- A second reservation returns `ErrPushDispatchAlreadyReserved` and **no envelope**, with
  a durable duplicate-reason audit. Non-eligible reservations fail closed and are audited.
- A provider timeout after reservation creates an **unknown external outcome**.
  Never remove the reservation or silently re-send on a timeout. Provider idempotency/receipts,
  reconciliation and an explicit operator decision are needed to safely recover.
- This is not a replacement for `NotificationIntent`, not another client truth and does
  not hold sensitive consultation content in transport.
- Staging/production readiness requires the new reservation table; migration
  `000030_r2_notification_push_dispatch_reservations.sql` runs under the canonical runner.

## Automated proof

`TestNotificationTransportClaimIsIdempotentAuditedAndOwned` runs against migrated PostgreSQL,
including eight concurrent dispatch attempts: exactly one receives a provider-ready envelope,
seven are rejected, all attempts have audit evidence and one durable reservation exists.
A subsequent stale PENDING plan is also rejected. Domain tests retain the explicit
forbidden-state checks and verify the reservation boundary.

## Release limits

This is a correctness/security barrier, **not** proof of production APNs/FCM delivery,
device receipt, background routing, live provider reconciliation, or rollout.
The requirement must not be promoted to RELEASED on this evidence alone.
