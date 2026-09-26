# Release Evidence

Evidence is added only when it is reproducibly bound to Requirement ID, commit SHA, build/release identity, contract/schema version and environment.

Screenshots alone are not sufficient evidence for critical backend invariants. Bootstrap code does not mark requirements VERIFIED or RELEASED.


## Staging runtime evidence

`staging-runtime-evidence-v1` records reproducible staging-only proof bound to an exact deployed SHA. It may satisfy requirements whose acceptance explicitly permits E2E or staging proof, but it MUST keep `production_release=false` and MUST NOT be used to claim production restore, signed mobile/store evidence, rollout approval, or production release readiness.
