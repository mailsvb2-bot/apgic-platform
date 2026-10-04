# R0 evidence — APGIC-MOBILE-003 multi-surface verification

Requirement: `APGIC-MOBILE-003` — launch-critical R0–R4 completion must not become WEB-only.

## Canonical proof

`tools/multisurface_verification_e2e.py` exercises the real `tools/multisurface_guard.py` against an isolated copy of the repository.

The proof performs both sides of the release-state contract:

1. It changes `APGIC-MOBILE-003` to `VERIFIED` with no `surface://` consumer evidence and requires the real guard to fail with the missing-surface diagnostic.
2. It adds explicit WEB, IOS, and ANDROID surface evidence and requires the same guard to pass.

The test is run as an explicit mandatory step in the Canon / architecture CI job. It therefore proves the fail-closed transition rule rather than only linting the current registry snapshot.

## Scope

This evidence closes the `E2E_OR_STAGING_PROOF` class for the multi-surface completion invariant. It does not mark the requirement `RELEASED`; release status remains governed by the Requirement Registry and actual launch evidence.


## Exact verified candidate

CI run `37226468814`:
- Web surface: job `111507038423`
- iOS surface: job `111507038525`
- Android surface: job `111507038498`
- Multi-surface contract guard: job `111507038462`
- Canon / architecture conformance: job `111507038471`

The canonical evidence map reports `unproven_evidence: []`; APGIC-MOBILE-003 is now **VERIFIED**.
