# R0 ORG-002 sold direction history proof

This slice extends the Organization archive invariant into the real durable checkout path.

## Canonical chain

- Product stores `organization_direction_id`.
- Booking slot stores `product_id`.
- Immutable Order stores both `product_id` and `organization_direction_id`.
- Booking remains linked to Order through the existing `booking_id`.
- Ledger remains linked to Order through the existing `economic_event_ref`.
- Audit evidence may reference the archived direction and remains append-only.

## Runtime

`backend/internal/runtimepostgres/journey.go` creates/validates the conformance Organization, ACTIVE membership/ownership, direction and Product inside the same checkout transaction before Booking/Order creation. Any canonical mismatch fails the whole transaction.

## Evidence

`backend/internal/runtimepostgres/journey_test.go` exercises the real durable journey:

1. creates a checkout;
2. persists Product/Direction refs in the immutable Order;
3. captures payment and creates Ledger history;
4. persists direction archive audit evidence;
5. archives the sold direction;
6. proves hard delete is rejected;
7. proves Product, Order, Booking, Ledger and Audit history still resolve after archival.

The database migration remains compatible with pre-existing legacy Order rows by allowing the historical pair `product_id=NULL, organization_direction_id=NULL`; new application Order snapshots require both refs at the domain boundary.

## Store entitlement preservation

`backend/migrations/ci_r2_mobile_store_invariants.sql` now uses the same canonical Product/Direction model for the real store-purchase invariant:

- the Booking slot references the canonical Product;
- the immutable Order snapshots Product and Direction;
- store verification uses the immutable Order Product UUID as `product_ref`;
- the entitlement is activated from that verification;
- the Direction is archived;
- direct hard delete is rejected;
- the entitlement remains ACTIVE and resolves through Verification → Order → Product → archived Direction.

At the code/test level this closes the entitlement-history gap in the APGIC-ORG-002 acceptance path. Fresh deployed staging evidence is recorded in `canon/evidence/staging-org-product-20261007T125100Z.json`, and installed iOS/Android app evidence is bound from CI run `37628404273`. With those external/runtime proofs attached, APGIC-ORG-002 is `VERIFIED`.
