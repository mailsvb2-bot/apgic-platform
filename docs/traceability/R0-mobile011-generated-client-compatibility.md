# APGIC-MOBILE-011 — Generated client contract compatibility

Status: **VERIFIED**.

## Acceptance

OpenAPI/schema changes must remain compatible with generated Web/iOS/Android consumers, or a governed version/deprecation/migration path must exist.

## Canonical implementation

- `contracts/openapi/apgic-v1.yaml`
- `packages/contracts/src/generated/apgic-v1.ts`
- `tools/generate_client_contract.py`
- `canon/contracts/client-surface-manifest.yaml`
- Web and native client contract consumers

## Exact evidence

CI run `37226468814`:
- Web build/consumer proof: job `111507038423`
- iOS native build/consumer proof: job `111507038525`
- Android native build/consumer proof: job `111507038498`
- React Native typecheck/generated-client proof: job `111507038445`
- Canon contract/generation guard: job `111507038471`

The canonical evidence map reports `unproven_evidence: []` for APGIC-MOBILE-011.
