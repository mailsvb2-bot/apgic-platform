# APGIC-MOBILE-029 — Native accessibility and MobileSupportPolicy

Status: **VERIFIED** — exact-candidate native evidence is green and bound to the Requirement Registry.

## Canon requirement

APGIC-MOBILE-029 requires VoiceOver/TalkBack-compatible native accessibility semantics, text scaling, reduced-motion awareness, deterministic focus order, minimum touch targets, and an explicit supported OS/device policy. Required evidence is:

- IOS_A11Y_TEST
- ANDROID_A11Y_TEST
- DEVICE_MATRIX_EVIDENCE

## Implementation path

The installed React Native app exposes a debug-only accessibility E2E probe when the native launch flag `APGIC_E2E_ACCESSIBILITY` is enabled. Production/default launches do not render the probe.

The probe contains:
- native accessibility labels and `button` roles for two ordered critical actions;
- 48dp minimum action height/width, exceeding the 44dp support-policy minimum;
- explicit font scaling;
- live native reduced-motion preference observation through `AccessibilityInfo`;
- deterministic source/accessibility order for primary then secondary action.

Android wires the E2E flag through `MainActivity`. iOS wires it through `AppDelegate`.

## Android installed-app proof

`tools/mobile_android_capability_e2e.sh` launches the built APK and captures the Android accessibility tree through `uiautomator`.

The proof fails closed unless:
- both critical actions are exposed by the native accessibility tree;
- both are clickable and enabled;
- both meet the 44dp minimum after converting policy dp to runtime pixels;
- primary precedes secondary in accessibility-tree order;
- font-scale and reduced-motion runtime markers are exposed.

The same installed APK is then exercised across representative window profiles:
- PHONE_COMPACT — 720x1280 @ 320 dpi;
- PHONE_LARGE — 1080x2400 @ 420 dpi;
- TABLET — 1600x2560 @ 320 dpi.

The resulting device-matrix record is written to `evidence/android-device-matrix.json`.

## iOS installed-app proof

`tools/mobile_ios_capability_e2e.sh` launches the built simulator app and captures the native accessibility hierarchy through IDB AXBridge.

The proof fails closed unless:
- both critical actions have the expected AX labels;
- AXBridge exposes button semantics;
- primary precedes secondary in the accessibility hierarchy;
- the same actions remain accessible after switching the simulator to an accessibility text-size category.

## Evidence artifacts

Candidate CI is expected to publish:
- `evidence/android-accessibility-e2e.xml`
- `evidence/android-a11y-phone_compact.xml`
- `evidence/android-a11y-phone_large.xml`
- `evidence/android-a11y-tablet.xml`
- `evidence/android-device-matrix.json`
- `evidence/ios-accessibility-e2e.json`
- `evidence/ios-accessibility-large-text.json`

Exact candidate evidence from CI run `37196979155`:
- Android native accessibility + device matrix: job `111420783059`
- iOS native AXBridge + accessibility text scaling: job `111420783056`
- Canon / architecture conformance: job `111420783250`
- final R0 bootstrap gate: job `111422569772`

All four gates passed. APGIC-MOBILE-029 is therefore **VERIFIED**.
