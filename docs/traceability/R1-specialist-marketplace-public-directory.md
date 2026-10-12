# APGIC R1 — Specialist Marketplace: public discovery and selection

**Canonical source:** APGIC v7 FINAL, especially sections 12–31, 226–231, 296, 299–302.
**Release slice:** R1 Supply-Min and Marketplace Discovery. Changes remain isolated in `feat/specialist-marketplace`, not merged into main.

## One complete user path, no invented provider state

1. The specialist signs in through the existing signed single Identity and declares their profession and capabilities in `/specialist`.
2. Evidence is submitted to the existing canonical owner; only trusted specialist review may approve an evidence subject and profile.
3. Only a complete, approved specialist with verified, non-expired capabilities and an eligible versioned qualification may publish.
4. Owner-entered public biography, experience, languages and scope limitations are durable in PostgreSQL; a changed public claim invalidates prior approval and atomically suspends active publications, pending fresh review **and explicit republish**. No self-verification.
5. `GET /v1/specialists` and `GET /v1/specialists/{id}` derive public projection from the canonical publication, capability, evaluation and profile state on every read. An unpublished specialist returns 404. No raw review evidence or Identity ID enters the public projection.
6. `/specialists` offers actual topic-filtered discovery and `/specialist/{id}` shows the published page. A visitor can carry a preference to the existing HelpIntent flow; preference alone never implies match, pricing, availability, or bookability.
7. Native iOS/Android use the same versioned API and generated DTO types and re-read the card before selecting. No independent native specialist database.
8. Current consultation price, slots, booking and PSP checkout remain under existing product/offer and booking owners. There is no demo price or fabricated provider/appointment.

## Traceability

| Canonical requirement | Implementation | Proof |
| --- | --- | --- |
| APGIC-ID-001 / APGIC-AUTH-001 | Existing signed session + same owner Identity, `httpapi/public_specialist.go` | `httpapi/public_specialist_test.go` |
| APGIC-SPEC-001 / APGIC-SPEC-002 | Existing evidence/review/publication state + `runtimepostgres/public_specialist.go` | `runtimepostgres/public_specialist_integration_test.go` |
| APGIC-QUAL-001 / APGIC-QUAL-002 | Active ELIGIBLE versioned qualification and nonexpired verified competence required in every public read | PostgreSQL integration test and existing qualification tests |
| APGIC-SEARCH-001 | Query-scoped projection; PostgreSQL remains canonical, no separate public truth | Public directory read and immediate unpublish/403/404 checks |
| APGIC-MATCH-001 / APGIC-RANK-001 | Existing confirmed HelpIntent match ranking unchanged; catalog preference cannot manufacture eligible match | `apps/web/app/journey.tsx`, Playwright selection proof |
| APGIC-UI-001 / APGIC-MOBILE-* | Responsive web, native discovery and accessible selection | `apps/web/e2e/specialist-marketplace.spec.ts`, `mobile-specialist-directory.test.ts` |
| APGIC-DATA-001 | Explicit public DTO whitelisting, private evidence never transmitted | Public API tests, native response validation |

## Guardrails and intentional scope

- R1 does **not** fake payment acceptance, financial transactions, slot availability, reviews, reputation, certificates, or qualifications. Existing R2–R4 owners remain authoritative.
- A published profile does not automatically imply an active, priced and bookable offer.
- Existing protected `/specialists/{id}` mobile deep-link route is retained; publicly shareable profile pages use `/specialist/{id}` to avoid changing protected deep-link semantics.
- Public pagination is bounded; unknown or unsupported filters are rejected rather than returning unauthorized profiles.
- Operative verification is still executed only by the existing trusted review operator; publishing directly via the public specialist API is impossible without it.
- A real production/staging release requires fully green CI, PostgreSQL migration deployment with existing migration ledger, and a live R1 end-to-end proof against actual persisted specialist data. Green local/unit results alone are not release proof.

## Actual price and availability projection

The public specialist page uses `GET /v1/specialists/{id}/offers`. The response is
derived solely from active canonical `specialist_bookable_offers`, the current
`apgic_live_offer_valid` PostgreSQL eligibility predicate, a published
Organization/Product, and future exclusive slots without an active hold or
booking. It is capped and does not expose client Identity, provider credentials,
private verification files, or mutable booking entitlements.

A slot disappearing from public results after a hold is an expected business
invariant, not an error: the existing confirmed HelpIntent + authoritative hold
path must be used to reserve a slot. The booking API revalidates availability;
clicking a time in the catalogue never creates a reservation or confirms payment.

**Proofs:** `TestLivePricedSpecialistToDurableHoldAfterRestart` now verifies
public offer price, slot, and disappearance after hold. HTTP tests cover
non-public 404, disabled caching and sensitive-field non-disclosure. Browser
and native clients both use the versioned `PublicBookableOfferList` schema.

This projection must never replace the canonical booking or PSP owner.

## Qualification revocation, provenance-aware ranking and selection

A trusted operator can execute `specialist-review -type capability -id PROFILE_UUID -topic anxiety -decision revoke -reviewer OPERATOR`. The capability and publication are revoked atomically with audit history. The PostgreSQL E2E verifies public directory, public offers, live matches and new booking holds all fail closed immediately while preserving already-held booking history.

Migration `000033` ties a live priced offer to the immutable ELIGIBLE qualification evaluation, current policy, jurisdiction, online format and adult age scope. Publishing additionally tests evidence expiry.

Real live ranking derives reliability and platform experience only from canonical COMPLETED and NO_SHOW bookings; current exclusive available slot count supplies the availability tier. There is no authoritative verified rating/review feed yet; `ReputationTier` is deliberately zero rather than fabricated, as is `CommercialBoostTier` until a trusted commercial placement owner exists. Rank also rejects non-ELIGIBLE input irrespective of sponsored boost. Unit tests cover this explicit invariant.

Public offer links pass both `specialist_id` and `offer_id` as *preferences* into the confirmed-HelpIntent workflow. The server re-evaluates qualification, current offer ownership, price and slot before any hold; marketplace does not execute payment or bypass booking. MatchCard prices resolve via offer ID, preventing wrong quoted price when a specialist has multiple services.

PostgreSQL integration test `TestPublicSpecialistFullPublicationReviewAndRemoval` additionally proves that a new piece of qualification evidence immediately withdraws the old published topic from public profile and search, and neither evidence review nor profile review alone resurrects the prior publication. Explicit policy evaluation and republish are required before discovery resumes.

## Native exact-service handoff and SQL supply ownership

Native specialist cards now expose a separate selection action for each real API-backed priced `offer_id`. The navigation URL carries the canonical specialist and offer UUIDs into the same confirmed HelpIntent workflow used on web; unpriced profiles cannot masquerade as a specific bookable offer. The URL validator rejects arbitrary or injected identifiers. TypeScript and native contract checks cover this path.

The PostgreSQL live-matching query places `slot.specialist_identity_id = specialist_profile.identity_id` on the **inner booking slot join**, not on the optional completed/no-show outcomes left join. Therefore the absence of booking history cannot accidentally relax canonical slot ownership. New specialist outcome metrics remain read-only ranking evidence, not a booking entitlement or money-processing module.
 
## Canonical independently moderated reputation

The PostgreSQL migration `000034_r1_verified_specialist_feedback.sql` creates one immutable rating per **completed, real specialist/live booking**, with exact specialist and client identity derived from canonical booking slots. A database BEFORE INSERT guard independently rejects unfinished, future-dated, cross-owner, wrong-specialist or otherwise fabricated reviews. The only client-write endpoint is `POST /v1/specialist-feedback` with a server-signed client session, canonical `booking_id`, and rating 1–5. Clients cannot send a specialist ID, published flag or reviewer decision. The public response reveals neither client identity nor booking ID.

A separate trusted CLI operator approves/rejects pending evidence: `specialist-review -type feedback -id REVIEW_UUID -decision approve -reviewer OPERATOR`. Decision, timestamp and reviewer are persisted and audited; score and source booking are immutable. Unapproved/rejected/withdrawn ratings never affect the directory or matching. No commercial boost can make a review trustworthy.

Public card and profile expose `verified_review_count` and **optional** `verified_rating` only once five reviews have been independently approved. Ranking uses the same verified counts and sum under a fixed neutral-prior smoothing formula, not self-reported years of experience, anonymous testimonials or commercial placement. Below five ratings, the rating tier remains zero/unrated. The booking lifecycle and payment execution stay exclusively with their preexisting canonical owners.

PostgreSQL integration denies a review from a genuine client with only a held slot, and rejects one from an unrelated identity. Go/Web/Native contract and unit tests additionally cover provenance, minimum samples, privacy and deterministic ranking. An end-to-end completed-session submission requires a real completed consultation and is intentionally impossible in the present live PSP fail-closed environment until payment execution is configured; it must not be simulated as production.
