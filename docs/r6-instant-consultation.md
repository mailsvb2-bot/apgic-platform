# R6 Duty/Now — instant consultation (APGIC canonical owner)

**Feature branch:** `feat/instant-consultation`. **Merge:** prohibited until integration review. **Release:** R6 is outside the v7 R0–R4 launch cut.

## Product journey and invariants

The UI entry points are `/now` (client), `/specialist/now` (specialist), and the native iOS/Android role-aware Duty/Now surface. All use the *same* Go API and PostgreSQL truth.

1. A client creates and **explicitly confirms** HelpIntent, with topics, audience, locale, jurisdiction and online format.
2. A specialist has an APGIC-reviewed, active, topic-specific publication, an explicitly priced real Product/Offer with a verified commercial owner, and opts into Duty/Now for at most ten minutes. The opt-in is **not a statement that a realtime room exists**.
3. APGIC selects a specialist using the canonical offer and latest eligibility decision; it will not dispatch to unqualified, self-declared, incompatible or already busy specialists. An atomic DB row lock (`FOR UPDATE SKIP LOCKED`) and partial unique indexes prevent double dispatch across processes.
4. One immutable **offered economic snapshot** (specialist, topic, offer, price/currency) is stored. The specialist has 45 seconds to accept or decline. The client polls the authorized request. Expiry and decline release the specialist, and the client may retry or book later.
5. Acceptance changes **Duty Dispatch** to `ACCEPTED` and holds the prepayment agreement for ten minutes. It does **not** create payment, booking, entitlement, communication session, payout, recorded medical facts, or join token. `next_action=PAY_REAL_PROVIDER_REQUIRED` and `room_ready=false` must remain true until an independently verified, non-custodial provider payment and booking/fulfillment bridge is integrated.
6. A reconnect or app restart recovers `GET /v1/instant/active` from durable PostgreSQL, with the signed session as the authorization source.

**State vocabulary**:
- Presence: `OFFLINE`, `AVAILABLE_NOW`, `OFFERED`, `RESERVED`, `IN_CONSULTATION`, `COOLDOWN`
- Dispatch: `UNFILLED`, `OFFERED`, `ACCEPTED`, `DECLINED`, `EXPIRED`
- The `IN_CONSULTATION` state is reserved for future R3/R6 fulfillment integration; this feature does **not** set it without verified paid booking evidence.
- `INVALIDATED` is a terminal state for an active offer/accepted-but-unpaid agreement whose real Product/publication/most recent qualification is no longer eligible. Availability becomes OFFLINE; the client is never sent to a payment or room based on stale approval.
- `CANCELLED` is a durable terminal state for the signed client's explicit cancellation before confirmed payment. It never means a PSP refund occurred. A second cancel returns the same terminal state; a late specialist acceptance is rejected.

## Ownership / API contract

The API is always scoped to the authenticated APGIC `ClientSession`, including when the same Identity is simultaneously a client and specialist. The client cannot supply an alternate owner. The only mutable inputs are confirmed intent UUID, UUID idempotency request key, selected existing offer UUID, and specialist explicit accept boolean.

| Endpoint | Meaning |
| --- | --- |
| `GET /v1/instant/request-key` | Issue a native-compatible UUID retry key; no local random API assumption |
| `PUT /v1/instant/presence` | Verified specialist opt in/out; conflict when scheduled, offered or reserved |
| `GET /v1/instant/presence` | Read own state and expiry |
| `GET /v1/instant/inbox` | Read only own incoming offers, not private HelpIntent text |
| `POST /v1/instant/dispatches` | Serialize and persist eligible client request; idempotent by client + key |
| `GET /v1/instant/dispatches/{id}` | Read client/specialist-owned proposal only |
| `GET /v1/instant/active` | Recover client-owned unexpired offer or accepted pending-payment agreement |
| `POST /v1/instant/dispatches/{id}/answer` | Specialist answer, only for owned active proposal |
| `POST /v1/instant/dispatches/{id}/cancel` | Signed client cancels pending offer or unpaid agreement; never claims a refund |

**Versioned response schema:** `contracts/jsonschema/instant-duty-v1.schema.json`. Non-custodial access gate is explicit. No raw consultation content is read or exported by this subsystem. Outgoing notifications are not invented; client and specialist poll durable state until a qualified messaging connector is available.

**Production flag:** `APGIC_DUTY_NOW_ENABLED=1` is an explicit opt-in; absence/any other value is disabled. Production requires the canonical PostgreSQL store and signed client sessions. No demo catalog, fake specialists, synthetic payment provider, self-claimed verification or assumed jurisdiction is used in Duty/Now.

## Failure/abuse matrix

| Case | Required behavior |
| --- | --- |
| No matching online qualified specialist | `UNFILLED`, clear booking-later option, no fabricated availability |
| Repeated request (same owner/key/intent) | Return original durable offer; no new economic effect |
| Same key, different HelpIntent | Conflict rather than silent retargeting |
| Two simultaneous clients / multiple Go replicas | PostgreSQL exclusive row lock + partial uniqueness; at most one active dispatch for specialist and client |
| Specialist declines or misses 45s window | Terminal decline/expiry; release to future dispatch, never silently accept |
| Specialist has overlapping paid/scheduled booking | Do not enter duty/select/accept conflicting instant offer |
| Qualification, Product publication or ownership revoked | Reject availability and acceptance; fail closed |
| New latest qualification becomes MANUAL_REVIEW between dispatch and acceptance | Automatically invalidate the offer, remove the specialist from live availability, reject late acceptance and refuse any room/payment claim |
| Fraudulent foreign UUID/identity/cross-tenant request | Session-scoped 404/403, no private resource disclosure |
| Time passes / app closes | Server expiry and ten-minute presence lease, not an infinite green badge |
| Payment not configured / uncertain provider callback | Never issue booking/entitlement/realtime room credentials |
| Restart / refresh during dispatch | Same persisted state is read; no new dispatch needed |
| Client cancels offered or accepted-but-unpaid proposal | Only signed owner may cancel, retry is idempotent, late specialist response denied; specialist returns to eligible supply only if lease and qualification remain valid |
| Client explicitly starts a new request after cancelling | May choose the same still-qualified specialist; a declined, timed-out or invalidated offer is not silently re-sent to that specialist for the same HelpIntent |
| Network breaks after request was committed | Reuse the same idempotency key in Web; native client recovers signed active dispatch and does not invent a second offer |
| Revoked qualification after previously successful offer | Real PostgreSQL journey revokes verified capability and confirms duty activation fails closed |

## Proof and integration gates

Go domain tests, HTTP owner/unsigned/missing-boolean/cancellation tests, SQL invariants, **a positive full real-Product/qualification-to-dispatch-to-cancellation journey (rolled back after proof)** and the dedicated migration are wired into canonical CI. Full PostgreSQL CI includes the new migration before invariant tests, including the parallel DB path. The feature is **not release-ready merely because source code compiles**.

Before enabling production, integration owners must prove, without changing the canonical owners:
- R2 booking/payment branch: specialist acceptance → mutually exclusive paid reservation → real PSP checkout/instruction → **verified** signed callback and ledger evidence → only then confirmed booking, entitlement; timeout/refund/cancellation follow existing business owners. APGIC does not receive or distribute funds.
- R3 consultation branch: provider-neutral credentialed join after verified booking entitlement, Ready Check, both-party joins, reliable session lifecycle, fulfillment evidence, interruption/recovery and post-session outcome.
- Durable outbox/notification worker and dedicated duty SLI/SLO/telemetry, provider down/degraded path, real concurrent PostgreSQL stress testing, mobile native build/e2e on supported devices.
- Release owner documents real contract/test/evidence refs and checks policy/version, age/qualification/jurisdiction, consent/privacy and manual review as appropriate. A green unit test is not a production-provider proof.

**Scope boundary:** This R6 feature deliberately does not implement bank/PSP onboarding, artificial payment confirmation, financial custody, or a competing parallel booking/consultation ledger. Those integrations must be performed against the canonical R2/R3 contracts (in other branches) only after their proof is available.
