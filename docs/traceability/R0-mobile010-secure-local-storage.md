# R0 evidence — APGIC-MOBILE-010 secure local storage and sensitive-data minimization

Requirement: APGIC-MOBILE-010 — native local storage must obey DataClass policy; raw consultation/persona/financial evidence must not become durable device state; persisted credentials must use approved system secure storage.

## Canonical storage boundary

apps/mobile/src/storage-policy.ts remains the canonical DataClass to storage-target decision:

- CREDENTIAL may persist only through SECURE_STORAGE.
- RAW_CONSULTATION, RAW_PERSONA and FINANCIAL_EVIDENCE may exist only in memory/ephemeral cache and are denied from persistent cache.
- ordinary direct AsyncStorage, MMKV, browser local/session storage, ad-hoc Android preferences/files and ad-hoc iOS defaults/files are rejected by repository audit.

The existing offline mutation queue remains a bounded 8192-byte audited INTERNAL persistence exception. Signed remote configuration remains bounded policy data, not user/session sensitive content.

## System credential storage

The native bridge now provides a production-capable credential boundary without adding a third-party storage dependency:

- Android encrypts credential bytes with AES-GCM using a non-exportable key generated and held by AndroidKeyStore; only ciphertext and IV are stored in app-private preferences.
- iOS persists credential bytes through Keychain generic-password items using kSecAttrAccessibleWhenUnlockedThisDeviceOnly, preventing sync migration of the credential item.
- CI simulator E2E remains a real Keychain runtime proof: the otherwise unsigned simulator artifact is ad-hoc signed with a dedicated CI-only application identifier/keychain access group, and the embedded entitlements are inspected before installation. Production signing/provisioning is not replaced by this CI identity.
- both platforms cap credential payloads at 4096 bytes and expose explicit load/save/clear operations through apps/mobile/src/secure-local-storage.ts.

## Logout/revoke purge

Both native implementations expose clearUserScopedState. It removes the user-scoped offline mutation queue and secure credential material. The production-capable revokeCurrentMobileInstallation boundary invokes that purge after an explicit current-device revoke and still purges if the server revoke attempt fails; logoutLocalSession uses the same purge boundary. Remote signed configuration is deliberately not user-scoped and remains available for last-known-safe incident behavior.

The existing installed-app Android/iOS installation E2E now writes a synthetic credential through the real native secure adapter, reads it back, performs current-device revoke, and proves the credential is absent afterwards. This turns logout/revoke retention from a static API claim into a device-runtime proof.

## Automated security proof

tools/mobile_native_storage_audit.py fails closed when:

1. a forbidden general-purpose persistence dependency is introduced;
2. direct native/local persistence appears outside the audited adapter;
3. Android backup is enabled;
4. the Android credential adapter loses Android Keystore / AES-GCM primitives, the bounded credential contract, or the session purge hook;
5. the iOS credential adapter loses Keychain primitives, ThisDeviceOnly accessibility, the bounded credential contract, or the session purge hook;
6. the JS secure-storage bridge stops exposing credential clear or user-scoped purge operations.

tools/tests/test_mobile_native_storage_audit.py includes negative regressions for ordinary preferences/defaults, weak secure-storage primitives, missing purge hooks and backup enablement.

Status remains IN_PROGRESS until the exact candidate commit passes repository CI and native Android/iOS builds. Only then may fresh run/job evidence advance the Requirement Registry to VERIFIED.
