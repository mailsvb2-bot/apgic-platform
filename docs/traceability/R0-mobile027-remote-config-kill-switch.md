# APGIC-MOBILE-027 — signed remote config, compatibility window and kill switches

Requirement: **APGIC-MOBILE-027**  
Release profile: **R0**  
Canon sections: **322, 330**  
Risk / priority: **HIGH / P0**

## Canon outcome

Installed iOS/Android clients must survive backend/config evolution through a documented compatibility window. Remote configuration is versioned, expiring and Ed25519-signed. It may disable runtime/presentation capabilities, but it cannot grant or own payment, entitlement, qualification, authorization, legal, ledger, payout or refund truth.

A config fetch failure uses an unexpired last-known-safe snapshot. If neither network nor valid cache is available, the native client fails safe by disabling every remotely gated capability rather than enabling one by accident.

## Contract and implementation

- Public API contract revision: `0.10.0-r0-remote-config`.
- Previous `0.9.0-r0-mobile-compatibility` remains in the supported contract window.
- `GET /v1/mobile/remote-config` returns a typed signed envelope with `key_id`, versioned payload, issued/expiry timestamps, policy id, disabled capability set and Ed25519 signature.
- Backend publisher rejects privileged business capabilities before signing.
- Native runtime cryptographically verifies the Ed25519 signature against a pinned/trusted public key for the declared `key_id`; unknown keys, signature tampering, expiry, rollback and same-version semantic drift fail safe before any gated capability can run. Accepted signed envelopes are persisted as last-known-safe.

## Last-known-safe and fail-safe

Android SharedPreferences and iOS NSUserDefaults use a dedicated bounded remote-config key, separate from the offline mutation queue. A process restart therefore preserves the accepted snapshot without creating business truth in local storage.

The native resolver order is:

1. load and validate unexpired cached snapshot;
2. fetch the canonical endpoint;
3. reject lower-version rollback and same-version semantic drift;
4. persist an accepted network snapshot;
5. on fetch failure use valid last-known-safe;
6. without valid cache, disable all remote-configurable capabilities.

## Deployment safety

Staging/production startup fails closed unless signed remote-config configuration is present. The deployment updater validates target-required environment keys before reset, validates remote-config key material/version/TTL, and re-executes the updater from the target commit so new preflight rules cannot lag one deployment behind.

No production private key is committed. CI generates ephemeral Ed25519 seed material at runtime.

## Automated evidence

- Go remoteconfig tests: signature tamper rejection, monotonic versioning, expiry, privileged-truth rejection, incident disable/recovery drill.
- HTTP test: endpoint returns an envelope cryptographically verifiable with the corresponding Ed25519 public key and fails closed when provider configuration is unavailable.
- Native tests: Ed25519 verification with trusted key pinning, signature-tamper/unknown-key rejection, network persistence, last-known-safe on fetch failure, rollback rejection, expiry and default fail-safe.
- Android/iOS installed-app E2E: the app pins the deterministic CI public key, verifies the backend signature, applies a realtime kill switch, and after process restart against an unavailable config endpoint recovers the same disabled state from signed last-known-safe without starting realtime.
- Compatibility tests prove previous supported contracts remain usable while the new API contract revision is introduced.

## Superseded candidate evidence

CI run `37101959533` was green for candidate commit `3764b59d11ce9e0c140277b570caf1875ecc0507`, but subsequent review found release trust-bootstrap, persistence-failure and expired-cache rollback gaps. That evidence remains historical and is not sufficient for the corrected candidate.

The corrected candidate additionally requires:

- release builds to receive a pinned trusted remote-config public key through governed build configuration;
- a cryptographically verified network kill-switch to apply for the current process even if last-known-safe persistence fails;
- the highest verified signed version to remain an anti-rollback high-water mark after its capability policy expires;
- a formal WEB compatibility exception because section 330 is an installed-native version/config capability, not a fabricated web flow.

## Corrected verified evidence

CI run `37114332039` completed successfully for corrected candidate commit `c764e31623d3a694d4c2a63a47269e040a62f1fe`.

- Canon / architecture gate `111178106947`: PASS, including multisurface verification and repository guard tests.
- Server / Go gate `111178107128`: PASS, including remote-config signing and emergency kill-switch drill.
- Android native gate `111178107111`: PASS, including installed-app remote-config network/last-known-safe and compatibility scenarios.
- iOS native gate `111178107001`: PASS, including installed-app remote-config network/last-known-safe and compatibility scenarios.
- Final R0 bootstrap gate `111181084063`: PASS.
- Release trust bootstrap is now wired for Android and iOS; production builds receive only pinned public verification material while private signing material remains outside the client/repository.
- Persistence failure no longer causes a verified network kill-switch to be ignored.
- Expired signed cache retains its verified version as an anti-rollback high-water mark while its expired capability policy is not applied.
- WEB is covered by formal compatibility exception because this requirement governs installed-native app version/config behavior.

Status: **VERIFIED**.
