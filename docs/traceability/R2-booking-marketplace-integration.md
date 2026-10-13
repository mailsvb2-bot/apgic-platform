# Canon v7 R2 — booking composed onto verified specialist marketplace

## Integration boundary

Candidate includes exact `main` at `b7d94441d6035acf4274c55268b97980814dba1c` and the R2 branch head `271eea16c70f8c4a2339b08530dc2d6a6ffc1626`. Both histories are retained. No R3/R6/R8/R9 or other feature branch is included.

The R1 migration ledger owns numbers `000032`–`000035`. R2 hold provenance uses `000036`, without internal transaction control; `apply-staging-migrations.sh` owns the atomic schema+ledger transaction. Its historical `help_intent_id` and `specialist_offer_id` are immutable after allocation. Retries are checked against original actor, confirmed intent, offer and slot; an occupied slot is never displayed as available, but same-origin idempotent readback must still work.

## Owner contracts

- Specialist and Qualification own verification/public eligibility, paid Product/Offer, pricing and slots. Publishing, withdrawal, concurrent disqualification and the stored R1 feedback/reputation proof are preserved.
- Booking owns exclusive hold, ordering, expiry, cancellation, verified payment evidence and durable readback through process restart. A checkout is an external provider instruction; a browser return cannot confirm capture.
- Real PSP evidence is required for paid entitlement and confirmed consultation. In its absence, the API/UI must fail closed, not invent a successful payment.
- The OpenAPI source and generated TypeScript client include both R1 public specialist routes and R2 booking readback contracts. Web and native surfaces must reject stale specialist/offer/slot selection.

## Release criteria

All tests must pass at the exact integrated head: Go formatting, vet and race, sequential PostgreSQL migrations and restore, R1 eligibility+revocation races, R2 hold ownership/replay and simultaneous clients, contract generation, desktop/tablet/phone Playwright, Android and iOS. Deployment / VERIFIED / RELEASED also require authentic external PSP callback proof and a live two-actor staging journey; mock/conformance tests alone cannot satisfy this. No synthetic transaction is represented as a real payment.