# APGIC-MOBILE-001 — Single business truth across web and native

Status: **VERIFIED**.

## Acceptance

Web/PWA/iOS/Android consume one backend/domain truth. Native clients do not own parallel booking, payment, qualification or ledger truth.

## Canonical implementation

- `canon/contracts/client-surface-manifest.yaml`
- `contracts/openapi/apgic-v1.yaml`
- generated client contract in `packages/contracts/src/generated/apgic-v1.ts`
- web and mobile clients consume the canonical generated contract
- `tools/multisurface_guard.py` rejects client-owned server-truth declarations and validates registered consumer surfaces

## Exact evidence

CI run `37210047640`:
- WEB build/E2E: job `111459211142`
- iOS native build/E2E: job `111459211117`
- Android native build/E2E: job `111459211183`
- multi-surface contract guard: job `111459211158`
- Canon / architecture conformance: job `111459211159`

The canonical evidence map reports no unproven evidence for APGIC-MOBILE-001.
