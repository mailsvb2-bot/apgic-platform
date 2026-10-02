# APGIC-MOBILE-013 — App version compatibility and safe update policy

## Canon requirement

Requirement `APGIC-MOBILE-013` (R0, P0) requires the backend to preserve supported installed app versions through a versioned compatibility window. A forced update is governed and is allowed only for security-, legal-, or critical-incompatibility reasons; an unsupported client must receive a user-safe update-required state without corrupting business data.

Canon section 322 defines `AppVersion`, `BuildNumber`, `min_supported_version`, and contract compatibility as versioned deployment policy and requires native E2E evidence in addition to build success.

## Canonical contract and implementation

- Current API contract: `0.9.0-r0-mobile-compatibility`.
- Supported previous contract used by the acceptance proof: `0.8.0-r2-offline-sync`.
- Explicit incompatible contract used by the negative-path proof: `0.7.0-unsupported`.
- `backend/internal/clientcompat/compatibility.go` owns deterministic version/build/contract evaluation.
- `backend/internal/httpapi/mobile_compatibility.go` exposes the fail-closed compatibility decision.
- `apps/mobile/src/mobile-compatibility-client.ts` validates the server decision before critical native runtime effects continue.
- `apps/mobile/src/App.tsx` blocks critical runtime effects on `UPDATE_REQUIRED` or compatibility-check failure and shows governed update UX for incompatible clients.
- `backend/cmd/api/main.go` requires a complete versioned compatibility policy in staging/production.

## Surface applicability

`APGIC-MOBILE-013` is an installed native-app compatibility requirement from Canon section 322. WEB has no native `AppVersion/BuildNumber` lifecycle under this requirement, so `canon/evidence/compatibility-exceptions.yaml` records approved exception `APGIC-MOBILE-013-WEB-NOT-APPLICABLE`. This avoids fabricating irrelevant WEB evidence while iOS and Android remain fully evidenced.

## Automated evidence

Candidate CI run: `37044947386` (run #1984) — SUCCESS.

### CONTRACT_COMPATIBILITY_TEST

PostgreSQL migration / invariant proof job `110963951025` passed the named step **Prove APGIC-MOBILE-013 contract compatibility and governed forced update**.

Coverage includes:
- supported previous contract remains non-blocking;
- version below minimum is governed;
- build below minimum is governed;
- unsupported contract returns `UPDATE_REQUIRED`;
- arbitrary forced-update reasons are rejected;
- missing policy fails closed;
- HTTP response preserves deterministic reason/policy/contract semantics.

Evidence ref:
- `surface://SERVER/mobile013-contract-compatibility/run-37044947386/job-110963951025`

### NATIVE_E2E — Android

Android native debug build job `110963951104` passed **Prove native capability and installation lifecycle on Android emulator**.

The installed app is launched with the supported previous contract and must complete the real canonical installation lifecycle through the backend (`installation-e2e:PASS`, revoked terminal state, push generation 2) without any `UPDATE_REQUIRED` state. The same installed app is then launched with the explicit unsupported contract and must expose the governed `UPDATE_REQUIRED / CLIENT_CONTRACT_UNSUPPORTED / INCOMPATIBLE_CRITICAL` update action.

Evidence ref:
- `surface://ANDROID/mobile013-native-e2e/run-37044947386/job-110963951104`

### NATIVE_E2E — iOS

iOS native simulator build job `110963951309` passed **Prove native capability and installation lifecycle on iOS Simulator**.

The iOS proof mirrors Android: the supported previous contract completes a real canonical installation lifecycle through the backend and the unsupported contract exposes the governed update-required UX.

Evidence ref:
- `surface://IOS/mobile013-native-e2e/run-37044947386/job-110963951309`

## Acceptance result

The supported previous installed-client contract continues to execute a real server-backed native critical flow after the backend contract update. An explicitly incompatible contract is blocked by a versioned, reasoned, user-safe forced-update path. Contract compatibility tests and iOS/Android native E2E are green. `APGIC-MOBILE-013` is therefore VERIFIED.
