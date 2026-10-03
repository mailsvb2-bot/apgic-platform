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

## Verified evidence

CI run `37101959533` completed successfully for candidate commit `3764b59d11ce9e0c140277b570caf1875ecc0507`.

- Server / Go gate `111143092958`: `TestEmergencyKillSwitchDrill` PASS.
- Android native gate `111143092945`: signed remote-config kill-switch + restart last-known-safe PASS; compatibility window PASS; native realtime restart/rejoin PASS.
- iOS native gate `111143093086`: signed remote-config kill-switch + restart last-known-safe PASS; compatibility window PASS; native realtime restart/rejoin PASS.
- Final R0 bootstrap gate `111144655308`: PASS.

Status: **VERIFIED**.
