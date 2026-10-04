# APGIC-MOBILE-022 — MobileDependencyRegistry and privacy supply-chain gate

Status: **VERIFIED**.

## Acceptance

Every native mobile SDK/dependency is registered with purpose, data classes, egress, permissions, provenance, privacy/security status and removal/kill strategy. Unregistered, stale, floating or privacy/security-incomplete runtime dependencies fail CI.

## Canonical implementation

- `apps/mobile/package.json`
- `apps/mobile/dependency-registry.yaml`
- `tools/mobile_dependency_guard.py`
- `tools/mobile_dependency_contract_guard.py`
- `tools/mobile_sbom.py`
- `tools/repository_secret_scan.py`

## Exact evidence

CI run `37210047640`:
- CycloneDX SBOM generation and mobile package tests: job `111459211162`
- dependency/privacy/secret-scan Canon gate: job `111459211159`
- iOS native build surface: job `111459211117`
- Android native build surface: job `111459211183`

The canonical evidence map reports all required APGIC-MOBILE-022 evidence types proven and `unproven_evidence: []`.

WEB does not use the native mobile SDK registry. The approved compatibility exception `APGIC-MOBILE-022-WEB-NOT-APPLICABLE` records that scope.
