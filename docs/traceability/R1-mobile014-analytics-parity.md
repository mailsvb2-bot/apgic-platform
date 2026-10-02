# APGIC-MOBILE-014 — Multi-surface analytics semantic parity

## Canon requirement

Requirement `APGIC-MOBILE-014` (R1, P0) requires one canonical analytics schema and one business meaning for the same user/business event across Web, iOS, and Android. Platform-specific diagnostics are allowed only as a namespaced extension. The Canon also forbids raw consultation transcript/media from becoming analytics payload.

## Canonical contract

- `packages/contracts/src/r1-mobile.ts` owns `CanonicalAnalyticsEventV1` and the canonical event-name set.
- `contracts/jsonschema/r1-mobile-cross-surface-v1.schema.json` is the machine-readable analytics schema.
- Required business fields are identical across surfaces.
- `platform_extensions` is optional but, when present, contains exactly one of `WEB`, `IOS`, or `ANDROID`.
- Raw transcript/audio/video payload keys are rejected by the analytics schema.

## Runtime consumers

- Web: `apps/web/src/analytics-client.ts` sends the event produced by `withWebAnalyticsDiagnostics`.
- iOS/Android: `apps/mobile/src/analytics-client.ts` sends the event produced by `withNativeAnalyticsDiagnostics`.
- The runtime transport is injected, so provider choice cannot redefine canonical business semantics.

## Automated evidence

CI run: `37001131994`.

### ANALYTICS_SCHEMA_TEST

Canon / architecture conformance job `110819152240` passed.

Evidence includes:
- `tools/tests/test_r1_analytics_schema.py`: positive schema validation for WEB/IOS/ANDROID and negative validation for multiple namespaces, raw transcript/media keys, missing required semantics, unknown events, and unknown top-level fields.
- `tools/r1_multisurface_guard.py`: contract/consumer anti-drift guard.

Evidence ref:
- `surface://SERVER/mobile014-analytics-schema/run-37001131994/job-110819152240`

### E2E_OR_STAGING_PROOF

Web / Next.js production build job `110819152528` passed the named step **Prove Web/iOS/Android analytics semantic parity** and the full Next.js production build/E2E suite.

`tools/r1_analytics_parity_e2e.mjs` sends the same `workspace_switched` canonical event through the Web, iOS, and Android runtime senders and asserts:
- identical event name and version;
- identical required business properties;
- identical journey/identity semantics;
- exactly one platform namespace per emitted event;
- no platform diagnostic field changes business meaning.

Evidence ref:
- `surface://WEB/mobile014-analytics-parity/run-37001131994/job-110819152528`

Native / React Native typecheck job `110819152520` passed TypeScript compilation and the native test suite, including the iOS/Android analytics sender tests.

Evidence refs:
- `surface://IOS/mobile014-analytics-parity/run-37001131994/job-110819152520`
- `surface://ANDROID/mobile014-analytics-parity/run-37001131994/job-110819152520`

Android native build/emulator E2E job `110819152505` also passed, proving the updated shared/native tree remains buildable and executable on Android.

Evidence ref:
- `surface://ANDROID/mobile014-native-build/run-37001131994/job-110819152505`

## Acceptance result

The canonical business event contract is shared across Web/iOS/Android, platform diagnostics remain namespaced, raw consultation content is excluded by schema, and executable parity/schema evidence is green. `APGIC-MOBILE-014` is therefore VERIFIED.
