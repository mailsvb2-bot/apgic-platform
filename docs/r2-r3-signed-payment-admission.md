# R2 → R3 signed payment admission

## Production boundary
The R3 communication owner (not the Duty/Now frontend and not payment
checkout) decides whether to issue provider room credentials for each signed
participant. For every `specialist/live` booking, the stored ACTIVE
entitlement and CONFIRMED booking are necessary but insufficient.

Migration `000049` adds the canonical database assertion
`apgic_live_consultation_payment_authorized(booking_id, basis_ref, now)`.
The assertion denies access unless *all* conditions agree:

- A real R2 order and SUCCEEDED attempt have the same amount/currency.
- A CAPTURED payment effect references a non-null economic ledger row.
- The PSP webhook receipt is signed/verified; the referenced ledger entry
  belongs to the *same order*, with matching economics and provider evidence.
- The participant's entitlement basis refers to that exact ledger entry.
- The booking remains CONFIRMED and inside its permitted session window.
- No REFUNDED effect for **any attempt of that order**; no ambiguous attempt.
- For R6 handoff bookings, the more restrictive accepted economic-snapshot
  payment proof also passes.

A missing or uncertain provider event must result in denied room admission,
not a network or billing fallback. R3 already checks both participants'
readiness independently and rechecks grants before persistence. This patch
never issues credentials, changes refunds, enables a mock PSP, or grants
access based on Duty/Now ACCEPTED.

## Verification
PostgreSQL CI runs the new migration and dedicated rejection invariant. The
existing R3 adapter tests, room issuer, and cross-surface jobs must also pass.

## Still required for production
Onboard a certified **real** PSP under existing R2 payment/control-plane
owners, configure its server-side keys and webhook validation, prove real
captured-payment → confirmed booking → distinct signed client/specialist
joins → actual provider STARTED/ENDED evidence, and verify refund/revoke
races. Keep R6 OFF until those proofs exist.
