# APGIC-PAY-004 — Separate provider / method / rail taxonomy

Status: **VERIFIED**.

## Canon requirement

PaymentProvider, PaymentMethod and PaymentRail are independent canonical concepts. A mixed `payment_type` field is forbidden because it collapses provider identity, user-visible method and regulatory/technical rail into one ambiguous value.

## Implementation

- `backend/internal/payments/taxonomy.go` defines separate typed `ProviderID`, `MethodCode` and `RailCode` values and a `Selection` containing all three fields.
- `contracts/openapi/apgic-v1.yaml` exposes `provider_id`, `method_code` and `rail_code` as separate required contract fields.
- `backend/migrations/000009_r2_payment_control_plane.sql` persists provider configuration separately from selected method and rail.
- `tools/payment_taxonomy_architecture_guard.py` scans production sources and rejects mixed `payment_type` semantics.

## Multi-surface contract proof

The canonical OpenAPI contract generates `PaymentSelection` in `packages/contracts/src/generated/apgic-v1.ts`. Web/PWA consume that generated contract through `apps/web/src/api-contract.ts`; iOS/Android consume the same generated contract through `apps/mobile/src/api-contract.ts`.

Therefore the taxonomy is one contract shared by WEB/PWA/IOS/ANDROID, not separate surface-specific business truth.

## Exact verification evidence

Fresh green `main` CI run **37358082684 / #2105** on candidate `a267ed77647f0d751d1366155ac7e6cbfe464dff` proved the unchanged PAY-004 implementation before this status-only evidence binding:

- Go format / vet / race tests — job `111925489621` — PASS.
- Canon / architecture conformance — job `111925490490` — PASS.
- Multi-surface contract guard — job `111925489775` — PASS.
- iOS native simulator build — job `111925489524` — PASS.
- Android native debug build — job `111925489677` — PASS.
- Web / Next.js production build — job `111925489688` — PASS.

The R0 CI evidence map already partitions every APGIC-PAY-004 required evidence class with `unproven_evidence: []`:
- `SCHEMA_CONTRACT_TEST`;
- `ENUM_REASON_TEST`;
- `ARCHITECTURE_GATE`.

## Verification rule

Any future change that reintroduces mixed payment taxonomy, changes the generated `PaymentSelection` contract, or causes WEB/PWA/iOS/Android to consume divergent payment taxonomy requires fresh verification before APGIC-PAY-004 may remain VERIFIED.
