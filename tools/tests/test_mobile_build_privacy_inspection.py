from __future__ import annotations

import plistlib
import tempfile
import unittest
from pathlib import Path

from tools.mobile_build_privacy_inspection import inspect_android, inspect_ios


ANDROID_MANIFEST = """<manifest xmlns:android="http://schemas.android.com/apk/res/android">
<uses-permission android:name="android.permission.INTERNET" />
<application android:allowBackup="false" />
</manifest>
"""


class MobileBuildPrivacyInspectionTests(unittest.TestCase):
    def test_ios_requires_bundled_manifest_matching_canonical(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            app = root / "APGIC.app"
            app.mkdir()
            manifest = {
                "NSPrivacyTracking": False,
                "NSPrivacyAccessedAPITypes": [],
                "NSPrivacyCollectedDataTypes": [],
            }
            canonical = root / "PrivacyInfo.xcprivacy"
            with canonical.open("wb") as handle:
                plistlib.dump(manifest, handle)
            with (app / "PrivacyInfo.xcprivacy").open("wb") as handle:
                plistlib.dump(manifest, handle)
            with (app / "Info.plist").open("wb") as handle:
                plistlib.dump({"CFBundleIdentifier": "com.apgic.ci"}, handle)

            result = inspect_ios(app, canonical)
            self.assertTrue(result["privacy_manifest_matches_canonical"])

            with (app / "PrivacyInfo.xcprivacy").open("wb") as handle:
                plistlib.dump({"NSPrivacyTracking": True}, handle)
            with self.assertRaises(ValueError):
                inspect_ios(app, canonical)

    def test_ios_rejects_missing_bundled_privacy_manifest(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            app = root / "APGIC.app"
            app.mkdir()
            canonical = root / "PrivacyInfo.xcprivacy"
            with canonical.open("wb") as handle:
                plistlib.dump({"NSPrivacyTracking": False}, handle)
            with (app / "Info.plist").open("wb") as handle:
                plistlib.dump({"CFBundleIdentifier": "com.apgic.ci"}, handle)
            with self.assertRaises(ValueError):
                inspect_ios(app, canonical)

    def test_android_compares_merged_permissions_and_backup_policy(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            manifest = root / "AndroidManifest.xml"
            manifest.write_text(ANDROID_MANIFEST, encoding="utf-8")
            data_safety = root / "data-safety.yaml"
            data_safety.write_text(
                "android_permissions:\n  - android.permission.INTERNET\n",
                encoding="utf-8",
            )
            result = inspect_android(manifest, data_safety)
            self.assertEqual(result["permissions"], ["android.permission.INTERNET"])
            self.assertFalse(result["allow_backup"])

            data_safety.write_text("android_permissions: []\n", encoding="utf-8")
            with self.assertRaises(ValueError):
                inspect_android(manifest, data_safety)

    def test_android_rejects_backup_enabled_in_merged_manifest(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            manifest = root / "AndroidManifest.xml"
            manifest.write_text(ANDROID_MANIFEST.replace('allowBackup="false"', 'allowBackup="true"'), encoding="utf-8")
            data_safety = root / "data-safety.yaml"
            data_safety.write_text(
                "android_permissions:\n  - android.permission.INTERNET\n",
                encoding="utf-8",
            )
            with self.assertRaises(ValueError):
                inspect_android(manifest, data_safety)

    def test_android_allows_known_debug_only_permissions_but_reports_them(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            manifest = root / "AndroidManifest.xml"
            manifest.write_text(
                """<manifest xmlns:android="http://schemas.android.com/apk/res/android">
<uses-permission android:name="android.permission.INTERNET" />
<uses-permission android:name="android.permission.ACCESS_LOCAL_NETWORK" />
<uses-permission android:name="android.permission.SYSTEM_ALERT_WINDOW" />
<uses-permission android:name="com.apgic.ci.DYNAMIC_RECEIVER_NOT_EXPORTED_PERMISSION" />
<application android:allowBackup="false" />
</manifest>
""",
                encoding="utf-8",
            )
            data_safety = root / "data-safety.yaml"
            data_safety.write_text(
                "android_permissions:\n  - android.permission.INTERNET\n",
                encoding="utf-8",
            )
            result = inspect_android(manifest, data_safety)
            self.assertEqual(result["product_permissions"], ["android.permission.INTERNET"])
            self.assertEqual(
                result["build_only_permissions"],
                [
                    "android.permission.ACCESS_LOCAL_NETWORK",
                    "android.permission.SYSTEM_ALERT_WINDOW",
                    "com.apgic.ci.DYNAMIC_RECEIVER_NOT_EXPORTED_PERMISSION",
                ],
            )

    def test_android_rejects_unknown_built_permission(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            manifest = root / "AndroidManifest.xml"
            manifest.write_text(
                ANDROID_MANIFEST.replace(
                    "<application",
                    '<uses-permission android:name="android.permission.CAMERA" />\n<application',
                ),
                encoding="utf-8",
            )
            data_safety = root / "data-safety.yaml"
            data_safety.write_text(
                "android_permissions:\n  - android.permission.INTERNET\n",
                encoding="utf-8",
            )
            with self.assertRaises(ValueError):
                inspect_android(manifest, data_safety)


if __name__ == "__main__":
    unittest.main()
