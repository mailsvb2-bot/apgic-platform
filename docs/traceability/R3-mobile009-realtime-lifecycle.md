# R3 evidence — APGIC-MOBILE-009 native realtime lifecycle and reconnect

Requirement: `APGIC-MOBILE-009` — native CommunicationProvider lifecycle must cover permissions, network transitions, reconnect, audio routes, background/foreground and interruption without creating false business transitions.

## Canonical runtime boundary

The implementation keeps native realtime state explicitly technical and separate from consultation business truth:

- `r3-realtime.ts` is the canonical reducer for session open, provider disconnect/reconnect, online/offline/degraded network state, Wi-Fi/mobile transport handoff, background/foreground, screen lock/unlock, microphone permission revoke/grant, audio-route changes, interruption, and join-auth expiry;
- every reduction emits `business_transition: NONE`; technical lifecycle events cannot mark a consultation completed or no-show;
- microphone revocation moves an active session to `BLOCKED`; online/foreground transitions cannot reconnect while permission is denied, and explicit permission grant is required before connection resumes;
- network transport changes reconnect through the provider boundary without changing consultation identity;
- background, screen lock and interruption pause/degrade media according to policy and reconnect only through the canonical controller;
- join-auth expiry issues `REFRESH_JOIN_AUTH`; refresh failure becomes `TECHNICAL_FAILURE`, not a business completion;
- `NativeRealtimeController` serializes lifecycle events and is the only executor of provider actions such as connect, reconnect, pause media, audio refresh, auth refresh and permission request;
- Android and iOS native bridges emit the same typed lifecycle contract consumed by React Native.

## Installed-app native lifecycle proof

Both native harnesses grant the real microphone permission, install and launch the built application, then exercise this lifecycle sequence:

`MICROPHONE_PERMISSION_REVOKED → NETWORK_OFFLINE → NETWORK_ONLINE → MICROPHONE_PERMISSION_GRANTED → NETWORK_TRANSPORT_CHANGED:CELLULAR → SCREEN_LOCKED → SCREEN_UNLOCKED → JOIN_AUTH_EXPIRED → APP_BACKGROUND → APP_FOREGROUND → AUDIO_ROUTE_CHANGED:BLUETOOTH → INTERRUPTION_BEGAN → NETWORK_ONLINE → INTERRUPTION_ENDED`.

The accessibility proof asserts:

- final phase `CONNECTED`;
- final network `ONLINE` on `CELLULAR`;
- final app state `FOREGROUND`, screen `UNLOCKED`, join auth `VALID`, and audio route `BLUETOOTH`;
- provider actions include initial connect, reconnect, media pause, audio-route refresh and join-auth refresh;
- every observed business transition is `NONE`;
- the consultation id is preserved;
- after a real process terminate/force-stop, the installed app repeats the proof and rejoins with the same consultation id.

This closes both the `NATIVE_E2E` and `REALTIME_E2E` evidence classes on Android and iOS.

## Staging business-truth proof

The staging proof runs the real API against an isolated PostgreSQL database initialized only from production migrations. It creates and pays a booking, enters consultation presence, records a recoverable network loss, and recovers the same consultation.

It proves that:

- presence is `IN_PROGRESS`;
- a technical network failure moves the consultation to `RECOVERING`;
- recovery returns it to `IN_PROGRESS`;
- none of those lifecycle operations charges again or claims APGIC room ownership;
- an empty completion evidence reference is rejected with HTTP 409 / `CONSULT_EVIDENCE_REQUIRED`;
- only an explicit provider completion carrying evidence can move the consultation to `COMPLETED`.

The isolated staging database prevents earlier invariant tests from mutating the catalog fixture used by this proof, so the `STAGING_PROOF` is independent of test execution order.

## Automated proof

Candidate `72357b1f828cec1d4e048a97e7343da20ed83562` passed CI run `36986013279` (CI #1955).

Required evidence and supporting regression gates:

- **NATIVE_E2E / REALTIME_E2E / ANDROID** — job `110771090200` passed the installed-app lifecycle and restart/rejoin proof.
- **NATIVE_E2E / REALTIME_E2E / IOS** — job `110771090184` passed the installed-app lifecycle and restart/rejoin proof.
- **STAGING_PROOF / SERVER** — PostgreSQL job `110771090196` printed `APGIC MOBILE-009 staging realtime proof: PASS` against the isolated migrated database.
- **Native state-machine/type proof** — job `110771090488` passed React Native typecheck and unit tests.
- **Go / race regression** — job `110771090316` passed format, vet and race tests.
- **Canon / architecture guard** — job `110771089933` passed.
- **Web regression** — job `110771090354` passed production build and E2E.
- **Restore safety** — job `110771090314` passed the isolated PostgreSQL restore drill.
- **Multi-surface contract** — job `110771090560` passed.
- **Final bootstrap gate** — job `110775813139` passed.

Exact multi-surface evidence bindings:

- Web: `surface://WEB/mobile009-realtime-lifecycle/run-36986013279/job-110771090354`
- iOS: `surface://IOS/mobile009-realtime-lifecycle/run-36986013279/job-110771090184`
- Android: `surface://ANDROID/mobile009-realtime-lifecycle/run-36986013279/job-110771090200`
- Server/PostgreSQL: `surface://SERVER/mobile009-realtime-lifecycle/run-36986013279/job-110771090196`

Run artifacts bind the candidate to:

- staging realtime evidence artifact `11217836960`, digest `sha256:0eaa8f315246139baff9e68894c479b1f3d7dd7772c9719e0fed1c48b5ed986f`;
- Android native artifact `11217703169`, digest `sha256:6546451bed77bb5247f32eb9d9aa0b98f69b5f4e0749edc8a296c2a227f27e4c`;
- iOS simulator artifact `11217708301`, digest `sha256:54eceb1218eb903e801e085a84a9a1f93fe2defe7edce2ecd853b6f7d8022ef9`;
- restore-drill artifact `11216939339`, digest `sha256:d59d433ca8073b5c15a25b2549ccbd8d1853d1de347ff2d006788f3a9072985f`;
- mobile SBOM artifact `11217168230`, digest `sha256:e35f97477ff8bc36c597781a8b714238eec8c3b8a42cd32d1449c2fdc1e32d00`.

The Requirement Registry can therefore advance to `VERIFIED`. This does **not** claim `RELEASED`: production rollout, provider credentials, store signing and production monitoring remain separate release evidence.
