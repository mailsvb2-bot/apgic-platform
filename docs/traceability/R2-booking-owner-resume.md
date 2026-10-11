# R2 — Canonical booking resume (APGIC-BOOK-002, multi-surface)

## Broken user journey
After booking, `/bookings/{id}` used to require a token in tab-local
`sessionStorage`. Following a direct link or opening a new tab could deny
a client with a valid authenticated session access to their own booking.
This did not test ownership against the booking owner at the point of read.

## Canonical execution
1. The browser or native client opens `/bookings/{id}` / invokes
   `GET /v1/bookings/{id}` with the existing signed ClientSession cookie.
2. API derives the principal from the trusted session, **never** from a
   caller-supplied identity ID.
3. `demand.Service.ReadBookingStatus` loads the latest journey snapshot
   from the existing durable PostgreSQL owner, enforcing hold expiry
   before returning the projection.
4. A foreign booking UUID and an absent UUID both return NOT_FOUND.
   API responds with `Cache-Control: no-store`. Status, time window
   and hold reference are returned without provider credentials or raw
   consultation/payment evidence.
5. HELD/PENDING_PAYMENT remain **not confirmed**. Only canonical
   CONFIRMED/COMPLETED states render the existing read-only consultation
   result UI. No client-generated economic state exists.

## Automated evidence
- Go domain test: owner can read held/pre-checkout state, then pending
  state; another API-service instance and the original service both
  observe persisted external state changes, while a foreign principal
  learns no booking existence.
- Go HTTP test: session required, owner-only read, held and
  PENDING_PAYMENT truth, no-store cache, no unnecessary secrets.
- Playwright web: direct URL with empty sessionStorage; valid result,
  outage, mismatched body, cancellation, logout and refresh.
- Native TypeScript: same generated BookingStatus schema, strict scope
  verification and fail-closed network/session errors.

## Release limits
This closes the bookmarkable authenticated booking-read path, **not**
real checkout, payment provider enrollment, provider-issued room
credentials, production media transport, payout or phone push.
External money execution and consultation remain separate launch gates.
