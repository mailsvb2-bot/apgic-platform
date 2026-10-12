# R1→R2 — real specialist priced offer → exclusive PostgreSQL hold

Requirement trace: APGIC-SPEC-002, APGIC-QUAL-001, APGIC-MATCH-001,
APGIC-BOOK-001, APGIC-BOOK-002, APGIC-PROD-001.

**Status: IN_PROGRESS.** This slice is not payment or launch readiness.

## Actual ownership and user route

1. The specialist creates a real canonical Identity and professional profile,
   declares competency, submits evidence and receives trusted review.
2. The specialist publishes the reviewed topic through the existing
   PostgreSQL qualification and publication gate.
3. The same authorized Identity creates an Organization, consultation
   direction and Product with explicit owner/author/beneficiary references;
   the Product must be **PUBLISHED**, not DRAFT.
4. In the specialist WEB workspace, the signed user selects that existing
   published Product and publishes a specific positive RUB price for one
   reviewed topic. A reviewed profile by itself is **not** a paid offer.
5. The specialist adds a future exclusive slot. Existing
   `booking_slots` is the owner, with `specialist_offer_id` attached. A
   DB advisory lock prevents overlapping schedules across API instances.
6. The client confirms HelpIntent and receives a MatchCard only when
   the live PostgreSQL offer is valid, priced and has a free slot. MatchCard
   includes the offer identifier to prevent mixing prices or topics.
7. An exclusive Hold rechecks the latest qualification/publication/Product
   gate inside PostgreSQL. Even if an older browser still holds a card,
   revoked supply cannot gain a fresh Hold.
8. A new service process reloads live holds/bookings from PostgreSQL,
   including historical holds of subsequently paused offers, and
   suppresses foreign-owner reads.

## Money and media boundaries

The live checkout still **fails closed** with
`ErrLivePaymentUnavailable`/`PAY_REAL_PROVIDER_REQUIRED`. This
change does not create a PSP, process/custody user funds, synthesize
capture evidence or grant media credentials. Existing conformance
specialists, synthetic prices and demo slots remain excluded from LIVE.

## Lifecycle and safety

- Qualified topics are owner-versioned in `specialist_publications`.
- A booked topic requires an immutable positive-priced `specialist_bookable_offers`
  row that references an existing published Product and approved Identity.
- Price cannot be changed in place; the owner can pause the old offer and
  publish a new version. Historic holdings and bookings remain readable.
- Live slots and holds are never stored in a second in-memory business truth:
  ephemeral catalog projections are rebuilt from canonical PostgreSQL.
- All new mutations use the signed client session for specialist Identity.
- Invalid, expired, revoked, overlapping or foreign-owner actions fail closed.
- Conformance catalog and tenant scope retain their former behavior.

## Execution evidence

- Domain tests `backend/internal/demand/live_supply_test.go`:
  real price and offer/topic match, exclusion of incomplete cards,
  confirmation-gated Hold.
- PostgreSQL test
  `TestLivePricedSpecialistToDurableHoldAfterRestart` in
  `backend/internal/runtimepostgres/live_supply_test.go`:
  create/review/publish actual specialist + Organization/Product;
  disallow DRAFT product; publish real price; create slot;
  block overlap and foreign mutation; match live client;
  persist Hold; restart API; verify owner-only booking status;
  pause offer and preserve historical Hold; issue new priced version.
- Web and native CI must pass unchanged shared contracts and existing
  critical journeys. This record never substitutes for live production
  PSP capture, consultation/provider evidence and release gate.

## Remaining canonical blockers

- Certified, configured external PSP, real checkout, trusted signed capture,
  refund, reconciliation, payout and no-custody evidence.
- Certified communication provider and actual two-party media plus
  signed consultation lifecycle evidence.
- Production configuration, domain/legal policies, multi-surface proof,
  real mobile store/signing/release evidence, production readiness.
- Whole-canon section traceability, statuses and other R0–R5 gaps
  continue to be reported by `canon_execution_gap_inventory.py`.
