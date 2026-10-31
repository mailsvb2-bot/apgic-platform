# R3: participant readiness → verified provider lifecycle → persisted client outcome

## Owner boundary

- APGIC persists the participant **ready check** separately from provider **JOINED / STARTED / ENDED** facts.
- Only signed CommunicationProvider events may mutate the authoritative consultation lifecycle; a browser cannot fake attendance or completion.
- A client can append a bounded self-reported progress + matching-satisfaction observation **only after** provider-evidenced COMPLETED. Self-report is not a clinical outcome claim.
- The client outcome table is append-only, scoped to booking owner and never joined into growth exports. Raw audio, transcript and sensitive free text are not stored.
- Neither readiness nor outcome create payment attempts, refunds, or changes to ledger.

## Interfaces

- GET/POST `/v1/consultations/{bookingID}/ready`: authenticated client or booked specialist; POST within [start - 15m, end] for confirmed booking only; idempotent and durable.
- GET/POST `/v1/consultations/{bookingID}/outcome`: authenticated client only; POST requires previously completed canonical session; 16–128 char key; immutable result record; idempotent replay with same payload, conflict on drift.
- GET `/v1/consultations/{bookingID}/result`: signed-session read of durable provider projection for either actual booked participant (client or specialist). Missing/inaccessible returns identical 404.

## Release evidence / intentionally not claimed

Mapped to APGIC-CONSULT-001/002, APGIC-PRIV-001, APGIC-DATA-001, APGIC-COMM-001, APGIC-MOBILE-009. These changes do not establish live third-party room provisioning, media connectivity, provider credential issuance or actual mobile device verification. Existing verified provider webhook pipeline and separate conformance tests remain the sole owner of JOINED/ENDED evidence. Do not mark any requirement VERIFIED until staging media call and multi-surface proof exist.

Required next certification: configured provider capabilities and credentials, booked specialist+client ready and scoped join, real 1:1 media, signed STARTED/ENDED, verified read after process restart, forced disconnect/reconnect, technical failure refund/reschedule routing and native lifecycle proof.

## Database negative-proof matrix

`backend/migrations/ci_r3_consultation_outcome_invariants.sql` proves both-side readiness, repeat idempotency, lack of fabricated provider presence, foreign principal denial, append-only records, no unverified outcomes, no specialist impersonation, bounded ratings, and no raw consultation columns. The readiness and outcome tables are now launch readiness dependencies. This remains simulated CI evidence rather than a live media provider certification.

## R3 media owner + durable failure history

The primary booking state and provider-confirmed session state are updated transactionally on signed `ENDED`: `bookings.state=COMPLETED` iff the authoritative `consultation_sessions.state=COMPLETED`. Signed lifecycle facts, evidence references and recovery decisions remain append-only. A separate owner-authorized `GET /v1/consultations/{bookingID}/timeline` rehydrates the complete metadata-only history through a repeatable-read PostgreSQL snapshot; inaccessible bookings are indistinguishable from absent sessions. Cancelled and no-show bookings can still display an existing consultation history; the browser does not infer media completion.

The latest communication join preparation additionally requires **both** saved participant readiness acknowledgements; these never count as JOINED/STARTED. A terminal completed or technically failed session cannot issue new room grants even if its booking remains economically confirmed. The terminal guard at PostgreSQL insert enforces this beyond the HTTP endpoint.

The specialist's separate `/specialist/consultations/{id}` page reuses the canonical participant-authenticated readiness, join, result and timeline. No separate specialist lifecycle brain is introduced.

**Open real-world acceptance gap:** signing/writing tests plus simulated LiveKit events do not establish a two-human production call. A live SFU test with audio/video both ways, restart of API process and connector outbox, network drop/recovery, entitlement revocation, signed completion, cancellation/reschedule/refund by canonical owners and mobile interruption remain mandatory. No production launch or 100% coverage should be claimed until such evidence is stored.
