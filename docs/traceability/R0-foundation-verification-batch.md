# R0 Foundation verification batch

Candidate basis: `a267ed77647f0d751d1366155ac7e6cbfe464dff`  
Fresh green CI: run **37358082684 / #2105**.

This record captures the R0 requirements whose canonical R0 CI evidence map reported `unproven_evidence: []` for candidate `a267ed77647f0d751d1366155ac7e6cbfe464dff`. It is an evidence-batch record, not an independent status authority. Requirement status is owned by `canon/requirements/registry.yaml`; this document must never be read as promoting a requirement that the Registry still marks `IN_PROGRESS`.

## Requirements

- `APGIC-AUTH-001`
- `APGIC-AUTH-002`
- `APGIC-ORG-001`
- `APGIC-ORG-002`
- `APGIC-CONN-001`
- `APGIC-CONN-002`
- `APGIC-EVENT-001`
- `APGIC-AUDIT-001`
- `APGIC-LEDGER-001`
- `APGIC-PROD-001`
- `APGIC-DATA-001`
- `APGIC-NFR-001`
- `APGIC-UI-001`
- `APGIC-TEST-001`
- `APGIC-RELEASE-001`
- `APGIC-LEGAL-001`
- `APGIC-LEGAL-002`
- `APGIC-TERM-001`
- `APGIC-CANON-001`

## Exact green jobs on the unchanged implementation

- Go format / vet / race tests — job `111925489621` — PASS.
- PostgreSQL migration / invariant proof — job `111925489706` — PASS.
- PostgreSQL isolated restore drill — job `111925489517` — PASS.
- Web / Next.js production build — job `111925489688` — PASS.
- Native / React Native typecheck — job `111925489571` — PASS.
- Android native debug build — job `111925489677` — PASS.
- iOS native simulator build — job `111925489524` — PASS.
- Multi-surface contract guard — job `111925489775` — PASS.
- Canon / architecture conformance — job `111925490490` — PASS.
- R0 bootstrap gate — job `111934687341` — PASS.

## Verification rule

For every requirement above, `canon/evidence/r0-ci-evidence-map.json` maps the requirement's declared evidence classes to concrete proof refs and reported no unproven evidence for the recorded candidate. That fact is necessary evidence, but it is not sufficient by itself to override additional cross-surface, staging, production, store, configuration, or external proof requirements documented elsewhere.

A requirement becomes `VERIFIED` only when the Registry itself is updated with the required exact-candidate evidence. Any future implementation change affecting a verified requirement requires fresh evidence before that status can be relied upon for release promotion.


## Additional R0 evidence recorded in this tranche

The same tranche also records evidence for:

- `APGIC-PAY-004` — separate PaymentProvider / PaymentMethod / PaymentRail taxonomy, with shared generated contract across WEB/PWA/IOS/ANDROID. The Registry currently carries its verified status.
- `APGIC-SEC-001` — scoped service-principal credential lifecycle and connector enforcement, with staging evidence in `canon/evidence/staging-sec001-service-auth-20261005T195233Z.json`.
- `APGIC-DR-001` — isolated restore with machine-readable staging evidence in `canon/evidence/staging-restore-20261005T200143Z.json`.

No aggregate VERIFIED count is asserted here. The live count must be derived from `canon/requirements/registry.yaml`, because later cross-surface or production evidence can legitimately leave an evidence-rich requirement `IN_PROGRESS`.
