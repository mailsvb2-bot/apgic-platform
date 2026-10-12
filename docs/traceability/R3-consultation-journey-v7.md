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
