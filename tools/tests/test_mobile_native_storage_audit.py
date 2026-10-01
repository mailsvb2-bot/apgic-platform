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
    write(
        root,
        "apps/mobile/android/app/src/main/AndroidManifest.xml",
        '<application android:allowBackup="false"></application>',
    )
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
    write(root, "apps/mobile/src/App.tsx", "export const App = () => null;\n")
    write(root, "apps/mobile/android/app/src/main/MainActivity.kt", "class MainActivity\n")
    write(root, "apps/mobile/ios/APGIC/AppDelegate.swift", "final class AppDelegate {}\n")


class NativeStorageAuditTests(unittest.TestCase):
    def test_accepts_no_direct_persistence_and_backup_disabled(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            valid_fixture(root)
            self.assertEqual(validate_native_storage(root), [])

    def test_rejects_async_storage_runtime_dependency(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            valid_fixture(root)
            package = root / "apps/mobile/package.json"
            package.write_text(
                json.dumps({"dependencies": {"@react-native-async-storage/async-storage": "2.0.0"}}),
                encoding="utf-8",
            )
            errors = validate_native_storage(root)
            self.assertTrue(any("persistent-storage runtime dependencies" in error for error in errors))

    def test_rejects_direct_android_shared_preferences(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            valid_fixture(root)
            write(
                root,
                "apps/mobile/android/app/src/main/Unsafe.kt",
                'val prefs = getSharedPreferences("session", 0)\n',
            )
            errors = validate_native_storage(root)
            self.assertTrue(any("direct native/local persistence" in error for error in errors))

    def test_rejects_direct_ios_user_defaults(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            valid_fixture(root)
            write(
                root,
                "apps/mobile/ios/APGIC/Unsafe.swift",
                'UserDefaults.standard.set("token", forKey: "credential")\n',
            )
            errors = validate_native_storage(root)
            self.assertTrue(any("direct native/local persistence" in error for error in errors))


    def test_allows_only_marked_bounded_offline_queue_adapters(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            valid_fixture(root)
            marker = "APGIC_AUDITED_STORAGE_ADAPTER: INTERNAL_OFFLINE_MUTATION_QUEUE"
            write(
                root,
                "apps/mobile/android/app/src/main/java/com/apgic/ci/OfflineMutationStorageModule.kt",
                f"// {marker}\nval max = 8192\nval prefs = getSharedPreferences(\"queue\", 0)\n",
            )
            write(
                root,
                "apps/mobile/ios/APGIC/OfflineMutationStorage.m",
                f"// {marker}\nconst int max = 8192;\nNSUserDefaults *defaults;\n",
            )
            self.assertEqual(validate_native_storage(root), [])

    def test_rejects_unmarked_or_sensitive_audited_adapter(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            valid_fixture(root)
            write(
                root,
                "apps/mobile/android/app/src/main/java/com/apgic/ci/OfflineMutationStorageModule.kt",
                'val max = 8192\nval prefs = getSharedPreferences("queue", 0)\n',
            )
            errors = validate_native_storage(root)
            self.assertTrue(any("audited storage adapter marker missing" in error for error in errors))

        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            valid_fixture(root)
            marker = "APGIC_AUDITED_STORAGE_ADAPTER: INTERNAL_OFFLINE_MUTATION_QUEUE"
            write(
                root,
                "apps/mobile/ios/APGIC/OfflineMutationStorage.m",
                f"// {marker}\nconst int max = 8192;\nNSUserDefaults *defaults;\n// CREDENTIAL\n",
            )
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
