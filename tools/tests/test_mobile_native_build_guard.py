from __future__ import annotations

import json
import tempfile
import unittest
from pathlib import Path

from tools.mobile_native_build_guard import validate


class MobileNativeBuildGuardTests(unittest.TestCase):
    def fixture(self) -> tuple[tempfile.TemporaryDirectory[str], Path, dict]:
        temp = tempfile.TemporaryDirectory()
        root = Path(temp.name)
        files = {
            "apps/mobile/package.json": json.dumps({"dependencies": {"react-native": "0.87.1"}}),
            "apps/mobile/android/app/build.gradle": 'namespace "com.apgic.ci"\napplicationId "com.apgic.ci"\n',
            "apps/mobile/android/app/src/main/java/com/apgic/ci/MainActivity.kt": 'package com.apgic.ci\noverride fun getMainComponentName(): String = "APGIC"\n',
            "apps/mobile/android/app/src/main/java/com/apgic/ci/MainApplication.kt": "package com.apgic.ci\n",
            "apps/mobile/android/gradle/wrapper/gradle-wrapper.jar": "wrapper",
            "apps/mobile/ios/APGIC.xcodeproj/project.pbxproj": "PRODUCT_BUNDLE_IDENTIFIER = com.apgic.ci;\n",
            "apps/mobile/ios/APGIC/AppDelegate.swift": 'withModuleName: "APGIC"\n',
            "apps/mobile/ios/Podfile": "target 'APGIC' do\n",
            "apps/mobile/index.js": "APGIC\n",
        }
        for relative, content in files.items():
            path = root / relative
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text(content, encoding="utf-8")
        config = {
            "applications": {
                "ios": {"bundle_id": "com.apgic.ci"},
                "android": {"application_id": "com.apgic.ci"},
            }
        }
        return temp, root, config

    def test_valid_ci_native_graph_passes(self) -> None:
        temp, root, config = self.fixture()
        self.addCleanup(temp.cleanup)
        self.assertEqual(validate(root, config), [])

    def test_debug_signing_on_release_is_rejected(self) -> None:
        temp, root, config = self.fixture()
        self.addCleanup(temp.cleanup)
        gradle = root / "apps/mobile/android/app/build.gradle"
        gradle.write_text(
            gradle.read_text(encoding="utf-8") + "signingConfig signingConfigs.debug\n",
            encoding="utf-8",
        )
        errors = validate(root, config)
        self.assertTrue(any("signing material" in error for error in errors))

    def test_bundle_id_must_match_governance(self) -> None:
        temp, root, config = self.fixture()
        self.addCleanup(temp.cleanup)
        project = root / "apps/mobile/ios/APGIC.xcodeproj/project.pbxproj"
        project.write_text("PRODUCT_BUNDLE_IDENTIFIER = com.other.app;\n", encoding="utf-8")
        errors = validate(root, config)
        self.assertTrue(any("bundle identifiers" in error for error in errors))


if __name__ == "__main__":
    unittest.main()
