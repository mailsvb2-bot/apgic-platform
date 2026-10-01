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

This evidence closes the `NATIVE_E2E` evidence class after both native jobs passed on the merged implementation candidate.

Verification binding:

- implementation merge SHA: `25d9218a6d2cbb1b27dac17047b793eb663b7027`
- CI run: `36783400366` (`CI #1720`, push to `main`, conclusion `success`)
- Android native job: `110118946422` — build + installed-app emulator fallback proof, conclusion `success`
- iOS native job: `110118946335` — simulator build + installed-app fallback proof, conclusion `success`
- `mobile-typecheck` unit/typecheck job and `canon` capability-contract guard also concluded `success`

Multi-surface binding:

- iOS proof: `surface://IOS/native-capability-fallback/run-36783400366/job-110118946335`
- Android proof: `surface://ANDROID/native-capability-fallback/run-36783400366/job-110118946422`
- WEB is formally marked not applicable for this requirement in `canon/evidence/compatibility-exceptions.yaml`, because Canon section 318 defines APGIC-MOBILE-004 as the Native OS Capability Layer and its acceptance/evidence are explicitly native-only.

The Requirement Registry is therefore advanced to `VERIFIED`. This does **not** claim `RELEASED`: production rollout, signing/store approval, and production release evidence remain governed separately.
