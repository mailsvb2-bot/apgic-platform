# APGIC-MOBILE-012 — Native CI and signed release artifacts

Status: **IN_PROGRESS**.

## Gap closed in this change

The repository already had a mobile release evidence schema and release-governance checks, but there was no executable binding between a release evidence document and:

- the exact candidate commit SHA;
- the approved organization identifier;
- the governed iOS bundle/team identity;
- the governed Android application/developer identity;
- signed-artifact declarations and SHA-256 identities;
- external store-account and signing-audit references.

`tools/mobile_release_evidence_guard.py` now enforces that binding.

## CI proof

The Canon CI job creates a CI-only evidence document bound to the exact current PR/head SHA and executes the release-evidence guard against `config/mobile/r0-ci-release-governance.yaml`.

This is intentionally **CI mechanics evidence only**. The synthetic hashes and `ci://` references do not satisfy production signing or store publication requirements.

Regression tests cover:

- exact candidate SHA mismatch;
- governance identity mismatch;
- unsigned artifact rejection;
- malformed artifact digest rejection;
- production fail-closed behavior without approved production governance;
- production requirement for external `evidence://` store/signing references.

## Remaining required evidence

APGIC-MOBILE-012 remains **IN_PROGRESS** because the Canon evidence map still correctly reports these unproven classes:

- `IOS_BUILD_PROOF`
- `ANDROID_BUILD_PROOF`
- `RELEASE_EVIDENCE`

Closing them requires actual production-signed iOS/Android candidates and their real artifact hashes/evidence, not CI debug builds.

## External dependency boundary

The repository can validate and bind production release evidence, but it cannot manufacture organization signing identities, App Store / Play developer ownership, or production signing secrets. Those resources must exist externally and be supplied through governed secret/evidence references.
