# R0 evidence — APGIC-MOBILE-004 native capability fallback

Requirement: `APGIC-MOBILE-004` — typed device capability and permission model.

## Runtime path

The mobile shell now receives a canonical `MICROPHONE` capability state from the platform adapter at application launch:

- Android maps package-manager/permission state in `MainActivity.kt` into React Native initial properties.
- iOS maps `AVAudioSession` microphone permission state in `AppDelegate.swift` into the same initial properties.
- `App.tsx` consumes only the canonical typed state and delegates the decision to `decideCapability`; no device brand/model enters business logic.
- Production defaults are fail-safe (`NOT_REQUESTED`, `DENIED`, `UNAVAILABLE`, or `UNKNOWN`) and the UI does not mutate server business truth.

Debug-only launch overrides exist solely to make negative native states deterministic in CI.

## Native E2E proof

The existing native build jobs now continue into a real installed-app check:

- `tools/mobile_android_capability_e2e.sh` boots an Android emulator, starts Metro, installs the actual debug APK, launches the app in `DENIED`, `RESTRICTED`, and `UNAVAILABLE`, and asserts the accessibility tree exposes the matching typed state and fallback reason.
- `tools/mobile_ios_capability_e2e.sh` boots an iPhone Simulator, installs the actual built `.app`, launches the same three states, and verifies the accessibility tree through IDB.
- Both jobs retain the captured accessibility trees as CI evidence artifacts.

Expected fallback mapping:

| state | action | reason |
| --- | --- | --- |
| `DENIED` | `FALLBACK` | `PERMISSION_DENIED` |
| `RESTRICTED` | `FALLBACK` | `OS_RESTRICTED` |
| `UNAVAILABLE` | `FALLBACK` | `CAPABILITY_UNAVAILABLE` |

## Scope

This evidence is intended to close the `NATIVE_E2E` evidence class only after both Android and iOS CI jobs pass. The Requirement Registry remains `IN_PROGRESS`; this PR does not claim production/store release evidence.
