# APGIC-MOBILE-023 — Mobile privacy declarations match actual build

Status: **VERIFIED**.

## Acceptance

Apple privacy manifests/App Privacy and Google Data Safety declarations are verified against the actual native build, dependency registry and canonical privacy declaration. Undeclared native data/required-reason API mismatches fail the release gate.

## Canonical implementation

- `apps/mobile/ios/APGIC/PrivacyInfo.xcprivacy`
- `apps/mobile/android/data-safety.yaml`
- `apps/mobile/privacy-declaration.yaml`
- `tools/mobile_privacy_guard.py`
- `tools/mobile_privacy_contract_guard.py`
- `tools/mobile_build_privacy_inspection.py`

## Exact evidence

CI run `37210047640`:
- privacy declaration/contract guards: job `111459211159`
- iOS actual-build privacy inspection: job `111459211117`
- Android actual-build privacy inspection: job `111459211183`

The canonical evidence map reports all required APGIC-MOBILE-023 evidence types proven and `unproven_evidence: []`.

WEB has neither an Apple PrivacyInfo manifest nor Android Data Safety declaration under this requirement. The approved compatibility exception `APGIC-MOBILE-023-WEB-NOT-APPLICABLE` records that boundary.
