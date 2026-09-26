# Release Evidence

Evidence is added only when it is reproducibly bound to Requirement ID, commit SHA, build/release identity, contract/schema version and environment.

Screenshots alone are not sufficient evidence for critical backend invariants. Bootstrap code does not mark requirements VERIFIED or RELEASED.


## Staging runtime evidence

`staging-runtime-evidence-v1` records reproducible staging-only proof bound to an exact deployed SHA. It may satisfy requirements whose acceptance explicitly permits E2E or staging proof, but it MUST keep `production_release=false` and MUST NOT be used to claim production restore, signed mobile/store evidence, rollout approval, or production release readiness.


## Requirement-level CI candidate evidence

`release-evidence-v2` adds a fail-closed R0 mapping from Requirement ID to the exact CI evidence kinds that were actually proven by successful gates and traceability refs.

- `CI_PROVEN` means every evidence kind listed by that requirement in the R0 CI map was proven by the candidate CI run. It does **not** change the Requirement Registry status and is not equivalent to `VERIFIED` or `RELEASED`.
- `PARTIAL` means CI proved only a subset; `unproven_evidence` remains mandatory before the requirement can advance under its acceptance semantics.
- `UNPROVEN` means CI deliberately claims none of the required evidence kinds.
- Production-only, manual, signed-store, real dashboard, and production restore evidence MUST remain unproven until the corresponding external proof exists.
- The map is versioned in `canon/evidence/r0-ci-evidence-map.json`; every claimed proof ref must already belong to that requirement's implementation/contract/test traceability and every claimed gate must be green.
