# APGIC: real integration and user-path verification

Authority: frozen Canon v7 and requirement registry, Launch v1 = R0–R4 only. This is a coverage ledger, not a RELEASED declaration.

## Runtime acceptance process

The platform-readiness lane pins main and ten feature HEADs, then actually attempts ordered three-way Git merges in an ephemeral worktree. It records source SHA, owner, exact conflicting paths, migration-version duplicates, which stage stopped, and every downstream stage NOT_EXECUTED. Independently, it attempts each other lane against the same control SHA; this diagnoses direct collisions but does not prove multi-lane compatibility.

On the accumulated, genuinely merged prefix GitHub Actions executes Go backend tests, canonical OpenAPI-generated client parity, sequential PostgreSQL 16 migrations and SQL invariants, durable cross-tenant, booking/restart/outbox and signed callback tests, Next.js build, React Native typecheck, and Playwright on desktop, tablet and phone. A failing process is evidence to investigate, not a status to mask. A blocked merge is not a reason to claim subsequent journeys work.

No temporary candidate is pushed or deployed. Main and all source branches remain unchanged.

## Critical user and operational variant families

Every real trial must bind the exact candidate SHA, actor/tenant and correlation identity to request/response, DB state, provider event and persisted outcome. These are scenario partitions, not a claim that unbounded input/concurrency permutations can all be enumerated.

| Canon cut | Positive and negative state transitions | Owner and authoritative evidence |
| --- | --- | --- |
| R0 identity | anonymous/new vs returning user; multiple sessions; client↔specialist role switch; expired credential; cross-tenant read/write denial; missing step-up | platform-core: signed session, audit, no data leak on web/iOS/Android |
| R0 event/recovery | transaction commits then restart; duplicate/reordered outbox delivery; transient provider outage; disaster restore | platform-operations: PostgreSQL readback, no duplicated ledger effect, measured RPO/RTO |
| R1 supply | verified specialist publishes; qualification revoked/expired; hidden/draft specialist; direct saved profile URL; sponsored placement | specialist-marketplace: qualification and publication truth enforced by API and public discovery |
| R1 offer/time | correct service/currency; changed specialist after slot load; empty slots; price change; timezone boundary; revoked availability | marketplace + booking: only live eligible bookable offers and stale state resets |
| R2 exclusive hold | user holds slot; two concurrent users race; hold expires; same-key replay vs distinct-key retry; restart before booking | booking-complete: PostgreSQL unique/exclusive state and no phantom booking |
| R2 PSP/order | valid order→external payment→signed callback; callback before browser return; forged, missing, duplicate, reordered and delayed receipt; amount mismatch | booking + finance: exact provider receipt, one durable order and entitlement only after confirmation |
| R2 compensation | cancellation before/after payment; expired hold; full/partial refund; duplicate refund callback; provider outage and API restart | booking + finance: one reversal and compensating immutable ledger, truthful owner visibility |
| R2 cross-surface | web→iOS/Android resume; offline operation conflicts; reinstall/revoked device; delayed/replayed notification | booking + native: one canonical client and booking state across surfaces |
| R3 admission | no entitlement; unpaid booking; expired room key; wrong tenant; both participants ready; media provider unavailable | consultation-runtime: fail-closed issuer and actual provider room authorization |
| R3 delivery | two real participants; reconnect/device switch; signed provider completed/failed/no-show; duplicated/out-of-order lifecycle; restart | consultation-runtime: external CommunicationProvider receipts, durable outcome and readback |
| R4 money | beneficiary/commission; accepted/failed/replayed payout; refund after settlement; reconciliation gap and kill switch | finance-provider-integrations: noncustodial provider execution, reconciled append-only ledger |
| R4 native | signed Android/iOS release; native store purchase/reversal; entitlement replay; policy or region denial | mobile-release-operations: real store/build/entitlement evidence |
| R4 rollout | canary and alert; isolation and privacy probes; DB forward recovery; isolated production restore; rollback without reversing ledger facts | platform-readiness + operations: independently verified operational receipts and approvals |
| Post-launch | personal cabinet, AI persona, instant consultations, courses, group events and growth attribution | respective lane owners; not automatically included in R0–R4 release cut |

## Evidence levels

1. SOURCE_ASSEMBLY: Git merged source + compile and tests. No claim about deployed behavior.
2. CONTRACT_SIMULATION: Playwright route mocks and in-memory provider; verifies only interface/contract behavior.
3. POSTGRES_RUNTIME: real migrated DB with server state, negative probes and restart; not by itself PSP/media truth.
4. PRODUCTION_LIKE_E2E: compiled same-SHA API and devices with representative policy and real explicitly identified external sandbox/provider receipts.
5. RELEASED: strict Canon candidate-bound R0–R4 requirements, human/provider/store/operations verification, authorized canary and verified external outcome. Successful CI or VERIFIED cannot become RELEASED automatically.

No source of production external provider/store proof can be invented. Missing or incomplete integration is BLOCKED or NOT_EXECUTED, not success. A workflow initialization failure is a runner/configuration failure and must be distinguished from product failure. Named human owners are not assigned in the Canon: owner names above designate responsible teams only.
