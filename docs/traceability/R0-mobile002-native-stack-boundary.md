# APGIC-MOBILE-002 — Canonical native stack and shared-code boundary

Status: **VERIFIED**.

## Acceptance

Native iOS/Android use TypeScript + React Native, with shared code restricted to canonical client contracts/schemas/tokens/localization/analytics. Server business truth is not copied into the native client.

## Canonical implementation

- `canon/contracts/client-surface-manifest.yaml`
- `apps/mobile/`
- `packages/contracts/`
- generated API contract consumed from `packages/contracts/src/generated/apgic-v1.ts`
- `tools/multisurface_guard.py` validates native runtime/language dependencies, shared-code roots and forbidden client-owned truth declarations

## Exact evidence

CI run `37210047640`:
- iOS native build: job `111459211117`
- Android native build: job `111459211183`
- multi-surface contract guard: job `111459211158`
- Canon / architecture conformance: job `111459211159`

WEB is not an installed React Native surface for this requirement. The approved compatibility exception `APGIC-MOBILE-002-WEB-NOT-APPLICABLE` records that boundary instead of fabricating WEB native-stack evidence.

The canonical evidence map reports no unproven evidence for APGIC-MOBILE-002.
