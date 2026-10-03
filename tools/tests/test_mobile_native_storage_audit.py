from __future__ import annotations

import json
import tempfile
import unittest
from pathlib import Path

from tools.mobile_native_storage_audit import validate_native_storage


def write(root: Path, rel: str, content: str) -> None:
    path = root / rel
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(content, encoding="utf-8")


def valid_fixture(root: Path) -> None:
    write(root, "apps/mobile/package.json", json.dumps({"dependencies": {"react": "19.3.0", "react-native": "0.87.1"}}))
    write(root, "apps/mobile/android/app/src/main/AndroidManifest.xml", '<application android:allowBackup="false"></application>')
    write(
        root,
        "apps/mobile/src/storage-policy.ts",
        '''const RAW_CONSULTATION = "RAW_CONSULTATION";
const RAW_PERSONA = "RAW_PERSONA";
const FINANCIAL_EVIDENCE = "FINANCIAL_EVIDENCE";
const CREDENTIAL = "CREDENTIAL";
function credential(target: string) { return target === "SECURE_STORAGE"; }
''',
    )
    write(root, "apps/mobile/src/secure-local-storage.ts", "loadCredential saveCredential clearCredential clearUserScopedState\n")
    write(root, "apps/mobile/src/App.tsx", "export const App = () => null;\n")
    write(
        root,
        "apps/mobile/android/app/src/main/java/com/apgic/ci/OfflineMutationStorageModule.kt",
        '''// APGIC_AUDITED_STORAGE_ADAPTER: INTERNAL_OFFLINE_MUTATION_QUEUE
// APGIC_SECURE_CREDENTIAL_ADAPTER: SYSTEM_KEYSTORE_V1
val queueMax = 8192
val credentialMax = 4096
val prefs = getSharedPreferences("queue", 0)
val provider = "AndroidKeyStore"
val cipher = "AES/GCM/NoPadding"
val spec = KeyGenParameterSpec
fun saveCredential() {}
fun loadCredential() {}
fun clearCredential() {}
fun clearUserScopedState() {}
''',
    )
    write(
        root,
        "apps/mobile/ios/APGIC/OfflineMutationStorage.m",
        '''// APGIC_AUDITED_STORAGE_ADAPTER: INTERNAL_OFFLINE_MUTATION_QUEUE
// APGIC_SECURE_CREDENTIAL_ADAPTER: SYSTEM_KEYSTORE_V1
const int queueMax = 8192;
const int credentialMax = 4096;
NSUserDefaults *defaults;
kSecClassGenericPassword;
SecItemAdd;
SecItemCopyMatching;
SecItemDelete;
kSecAttrAccessibleWhenUnlockedThisDeviceOnly;
saveCredential;
loadCredential;
clearCredential;
clearUserScopedState;
''',
    )


class NativeStorageAuditTests(unittest.TestCase):
    def test_accepts_audited_system_secure_storage_and_backup_disabled(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            valid_fixture(root)
            self.assertEqual(validate_native_storage(root), [])

    def test_rejects_async_storage_runtime_dependency(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            valid_fixture(root)
            package = root / "apps/mobile/package.json"
            package.write_text(json.dumps({"dependencies": {"@react-native-async-storage/async-storage": "2.0.0"}}), encoding="utf-8")
            errors = validate_native_storage(root)
            self.assertTrue(any("persistent-storage runtime dependencies" in error for error in errors))

    def test_rejects_direct_android_shared_preferences(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            valid_fixture(root)
            write(root, "apps/mobile/android/app/src/main/Unsafe.kt", 'val prefs = getSharedPreferences("session", 0)\n')
            errors = validate_native_storage(root)
            self.assertTrue(any("direct native/local persistence" in error for error in errors))

    def test_rejects_direct_ios_user_defaults(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            valid_fixture(root)
            write(root, "apps/mobile/ios/APGIC/Unsafe.swift", 'UserDefaults.standard.set("token", forKey: "credential")\n')
            errors = validate_native_storage(root)
            self.assertTrue(any("direct native/local persistence" in error for error in errors))

    def test_rejects_weak_android_credential_adapter(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            valid_fixture(root)
            path = root / "apps/mobile/android/app/src/main/java/com/apgic/ci/OfflineMutationStorageModule.kt"
            path.write_text(path.read_text(encoding="utf-8").replace("AndroidKeyStore", "plain-storage"), encoding="utf-8")
            errors = validate_native_storage(root)
            self.assertTrue(any("secure credential adapter missing required system-storage proof" in error for error in errors))

    def test_rejects_syncable_or_incomplete_ios_keychain_adapter(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            valid_fixture(root)
            path = root / "apps/mobile/ios/APGIC/OfflineMutationStorage.m"
            path.write_text(path.read_text(encoding="utf-8").replace("kSecAttrAccessibleWhenUnlockedThisDeviceOnly", "kSecAttrAccessibleAfterFirstUnlock"), encoding="utf-8")
            errors = validate_native_storage(root)
            self.assertTrue(any("secure credential adapter missing required system-storage proof" in error for error in errors))

    def test_requires_logout_revoke_purge_hook_on_both_platforms(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            valid_fixture(root)
            for rel in (
                "apps/mobile/android/app/src/main/java/com/apgic/ci/OfflineMutationStorageModule.kt",
                "apps/mobile/ios/APGIC/OfflineMutationStorage.m",
            ):
                path = root / rel
                path.write_text(path.read_text(encoding="utf-8").replace("clearUserScopedState", "missingPurgeHook"), encoding="utf-8")
            errors = validate_native_storage(root)
            self.assertGreaterEqual(sum("secure credential adapter missing required system-storage proof" in error for error in errors), 2)

    def test_rejects_unmarked_or_sensitive_audited_adapter(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            valid_fixture(root)
            path = root / "apps/mobile/android/app/src/main/java/com/apgic/ci/OfflineMutationStorageModule.kt"
            path.write_text(path.read_text(encoding="utf-8").replace("APGIC_AUDITED_STORAGE_ADAPTER: INTERNAL_OFFLINE_MUTATION_QUEUE", "UNMARKED"), encoding="utf-8")
            errors = validate_native_storage(root)
            self.assertTrue(any("audited storage adapter marker missing" in error for error in errors))

        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            valid_fixture(root)
            path = root / "apps/mobile/ios/APGIC/OfflineMutationStorage.m"
            path.write_text(path.read_text(encoding="utf-8") + "\n// CREDENTIAL\n", encoding="utf-8")
            errors = validate_native_storage(root)
            self.assertTrue(any("must not reference sensitive DataClass markers" in error for error in errors))

    def test_rejects_android_backup_enablement(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            valid_fixture(root)
            manifest = root / "apps/mobile/android/app/src/main/AndroidManifest.xml"
            manifest.write_text('<application android:allowBackup="true"></application>', encoding="utf-8")
            errors = validate_native_storage(root)
            self.assertTrue(any("disable application backup" in error for error in errors))


if __name__ == "__main__":
    unittest.main()
