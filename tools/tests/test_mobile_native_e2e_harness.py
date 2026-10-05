from __future__ import annotations

import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]


class MobileNativeE2EHarnessTest(unittest.TestCase):
    def _read(self, name: str) -> str:
        return (ROOT / "tools" / name).read_text(encoding="utf-8")

    def test_remote_config_constants_are_declared_before_use(self) -> None:
        for name in (
            "mobile_android_capability_e2e.sh",
            "mobile_ios_capability_e2e.sh",
        ):
            text = self._read(name)
            for variable in (
                "COMPATIBILITY_BASE_URL",
                "COMPATIBILITY_CONTRACT_VERSION",
                "REMOTE_CONFIG_E2E_KEY_ID",
                "REMOTE_CONFIG_E2E_PUBLIC_KEY_BASE64",
            ):
                assignment = variable + '="'
                usages = (
                    '"$' + variable + '"',
                    '"$' + '{' + variable + '}"',
                )
                usage_positions = [
                    text.index(usage) for usage in usages if usage in text
                ]
                self.assertIn(assignment, text, f"{name}: missing {variable} assignment")
                self.assertTrue(
                    usage_positions,
                    f"{name}: missing {variable} usage",
                )
                self.assertLess(
                    text.index(assignment),
                    min(usage_positions),
                    f"{name}: {variable} is used before declaration",
                )

    def test_exit_cleanup_preserves_failing_status(self) -> None:
        for name in (
            "mobile_android_capability_e2e.sh",
            "mobile_ios_capability_e2e.sh",
        ):
            text = self._read(name)
            cleanup_start = text.index("cleanup() {")
            cleanup_end = text.index("}\ntrap cleanup EXIT", cleanup_start)
            cleanup = text[cleanup_start:cleanup_end]
            self.assertIn("local status=$?", cleanup)
            self.assertIn("trap - EXIT", cleanup)
            self.assertIn('exit "$status"', cleanup)

    def test_android_e2e_budget_and_new_evidence_are_bounded(self) -> None:
        workflow = (ROOT / ".github/workflows/ci.yml").read_text(encoding="utf-8")
        android = self._read("mobile_android_capability_e2e.sh")

        android_job = workflow[
            workflow.index("  mobile-android-build:"):
            workflow.index("  mobile-ios-build:")
        ]
        self.assertIn("timeout-minutes: 45", android_job)
        self.assertIn("evidence/android-demand-e2e.xml", android_job)
        self.assertIn("evidence/android-deletion-e2e.xml", android_job)

        for path in (
            "/sdcard/apgic-installation-e2e.xml",
            "/sdcard/apgic-workspace-e2e.xml",
            "/sdcard/apgic-demand-e2e.xml",
            "/sdcard/apgic-deletion-e2e.xml",
        ):
            self.assertEqual(android.count("dump_until_labels_visible " + path), 1)

    def test_demand_e2e_matches_user_help_intent_execution_path(self) -> None:
        app = (ROOT / "apps/mobile/src/App.tsx").read_text(encoding="utf-8")

        demand_effect_start = app.index(
            "      !demandE2EBaseURL ||",
            app.index("setDemandE2E({status: \"RUNNING\"})") - 500,
        )
        demand_effect_end = app.index(
            "  ]);",
            app.index("setDemandE2E({status: \"RUNNING\"})"),
        )
        demand_effect = app[demand_effect_start:demand_effect_end]
        self.assertNotIn("compatibilityAllowsRuntime", demand_effect)
        self.assertIn(
            "compatibilityBaseURL !== canonicalAPGICOrigin && !demandE2EBaseURL",
            app,
        )

    def test_demand_and_deletion_e2e_use_local_compatibility_gate(self) -> None:
        android = self._read("mobile_android_capability_e2e.sh")
        ios = self._read("mobile_ios_capability_e2e.sh")

        for function_name in ("assert_help_intent_confirmation", "assert_account_deletion"):
            android_start = android.index(function_name + "() {")
            android_end = android.index("\n}\n", android_start)
            android_block = android[android_start:android_end]
            self.assertIn('APGIC_E2E_COMPATIBILITY_BASE_URL "$COMPATIBILITY_BASE_URL"', android_block)
            self.assertIn('APGIC_E2E_CONTRACT_VERSION "$COMPATIBILITY_CONTRACT_VERSION"', android_block)

            ios_start = ios.index(function_name + "() {")
            ios_end = ios.index("\n}\n", ios_start)
            ios_block = ios[ios_start:ios_end]
            self.assertIn('SIMCTL_CHILD_APGIC_E2E_COMPATIBILITY_BASE_URL="$COMPATIBILITY_BASE_URL"', ios_block)
            self.assertIn('SIMCTL_CHILD_APGIC_E2E_CONTRACT_VERSION="$COMPATIBILITY_CONTRACT_VERSION"', ios_block)

    def test_android_realtime_script_finishes_with_audited_audio_route(self) -> None:
        text = self._read("mobile_android_capability_e2e.sh")
        marker = 'local events="'
        start = text.index(marker, text.index("assert_realtime_lifecycle() {")) + len(marker)
        end = text.index('"', start)
        events = text[start:end].split(",")
        self.assertEqual(events[-1], "AUDIO_ROUTE_CHANGED:BLUETOOTH")
        self.assertIn("INTERRUPTION_ENDED", events[:-1])


    def test_ios_simulator_boot_has_bounded_data_migration_recovery(self) -> None:
        text = self._read("mobile_ios_capability_e2e.sh")
        self.assertIn('["xcrun", "simctl", "list", "runtimes", "-j"]', text)
        self.assertIn('["xcrun", "simctl", "list", "devicetypes", "-j"]', text)
        self.assertNotIn("mapfile", text)
        self.assertIn("IOS_DEVICE_TYPES=()", text)
        self.assertIn("while IFS= read -r device_type; do", text)
        self.assertIn('"iPhone 16 Pro"', text)
        self.assertIn('if not name.startswith("iPhone") or "Air" in name:', text)
        self.assertIn('xcrun simctl create "$candidate_name" "$device_type" "$IOS_RUNTIME"', text)
        self.assertIn('xcrun simctl delete "$UDID"', text)
        self.assertIn("SIMULATOR_CREATED=1", text)
        self.assertIn("boot_simulator_with_recovery() {", text)
        self.assertIn("for boot_attempt in 1 2; do", text)
        self.assertIn('grep -q "Data Migration Failed" "$SIMULATOR_BOOT_LOG"', text)
        self.assertIn('xcrun simctl erase "$UDID"', text)
        self.assertIn(
            "failed clean boot after bounded migration recovery",
            text,
        )
        self.assertIn('cat "$supported_output" >&2', text)

    def test_ios_simulator_uses_xcode_adhoc_signing_for_keychain_runtime(self) -> None:
        workflow = (ROOT / ".github/workflows/ci.yml").read_text(encoding="utf-8")
        app = (ROOT / "apps/mobile/src/App.tsx").read_text(encoding="utf-8")

        self.assertIn("CODE_SIGNING_ALLOWED=YES", workflow)
        self.assertIn("CODE_SIGNING_REQUIRED=YES", workflow)
        self.assertIn("CODE_SIGN_IDENTITY=-", workflow)
        self.assertIn("Verify Xcode simulator signing before Keychain runtime proof", workflow)
        self.assertIn("codesign --verify --deep --strict", workflow)
        self.assertIn("ios-simulator-codesign.txt", workflow)
        self.assertIn("Signature=adhoc", workflow)
        self.assertNotIn("APGICCI000", workflow)
        self.assertNotIn("APGICSimulatorCI.entitlements", workflow)
        self.assertIn("secureCredentialStorage.save", app)
        self.assertIn("secureCredentialStorage.load", app)
        self.assertIn("clearUserScopedLocalState", app)

    def test_ios_realtime_initial_pass_has_bounded_ambient_degradation_recovery(self) -> None:
        text = self._read("mobile_ios_capability_e2e.sh")
        realtime = text[text.index("assert_realtime_lifecycle() {"):]
        self.assertIn("ambient_technical_degraded() {", realtime)
        self.assertIn("for initial_attempt in $(seq 1 3); do", realtime)
        self.assertIn('json_has_ax_label "$target" "realtime-phase:DEGRADED"', realtime)
        self.assertIn('json_has_ax_label "$target" "realtime-business-transition:NONE"', realtime)
        self.assertIn('json_has_ax_label "$target" "realtime-audio-route:BLUETOOTH"', realtime)
        self.assertIn('json_has_ax_label "$target" "realtime-network-transport:CELLULAR"', realtime)
        self.assertIn('if realtime_ready "$output"; then', realtime)
        self.assertIn("break 2", realtime)
        self.assertIn(
            "did not complete native realtime lifecycle proof after bounded ambient-degradation retries",
            realtime,
        )


    def test_workspace_role_switch_is_exercised_on_both_native_surfaces(self) -> None:
        android_script = self._read("mobile_android_capability_e2e.sh")
        ios_script = self._read("mobile_ios_capability_e2e.sh")
        android_activity = (
            ROOT
            / "apps/mobile/android/app/src/main/java/com/apgic/ci/MainActivity.kt"
        ).read_text(encoding="utf-8")
        ios_delegate = (
            ROOT / "apps/mobile/ios/APGIC/AppDelegate.swift"
        ).read_text(encoding="utf-8")
        app = (ROOT / "apps/mobile/src/App.tsx").read_text(encoding="utf-8")
        client = (
            ROOT / "apps/mobile/src/mobile-workspace-client.ts"
        ).read_text(encoding="utf-8")

        for script in (android_script, ios_script):
            self.assertIn("assert_workspace_switch() {", script)
            self.assertIn("workspace-e2e:PASS", script)
            self.assertIn("workspace-e2e-kinds:CLIENT|SPECIALIST|ORGANIZATION", script)
            self.assertIn("workspace-e2e-foreign-denied:true", script)
            self.assertIn("assert_workspace_switch", script)

        self.assertIn("APGIC_E2E_WORKSPACE_BASE_URL", android_activity)
        self.assertIn("workspaceE2EBaseURL", android_activity)
        self.assertIn("APGIC_E2E_WORKSPACE_BASE_URL", ios_delegate)
        self.assertIn("workspaceE2EBaseURL", ios_delegate)
        self.assertIn("runWorkspaceE2EFlow", app)
        self.assertIn("/v1/mobile/workspaces", client)
        self.assertIn("WORKSPACE_E2E_FOREIGN_SCOPE_ALLOWED", client)

    def test_mobile029_accessibility_runtime_is_exercised_on_both_native_surfaces(self) -> None:
        android_script = self._read("mobile_android_capability_e2e.sh")
        ios_script = self._read("mobile_ios_capability_e2e.sh")
        workflow = (ROOT / ".github/workflows/ci.yml").read_text(encoding="utf-8")
        app = (ROOT / "apps/mobile/src/App.tsx").read_text(encoding="utf-8")
        android_activity = (
            ROOT
            / "apps/mobile/android/app/src/main/java/com/apgic/ci/MainActivity.kt"
        ).read_text(encoding="utf-8")
        ios_delegate = (
            ROOT / "apps/mobile/ios/APGIC/AppDelegate.swift"
        ).read_text(encoding="utf-8")

        self.assertIn("accessibilityE2EEnabled", app)
        self.assertIn('accessibilityLabel="a11y-action-primary"', app)
        self.assertIn('accessibilityLabel="a11y-action-secondary"', app)
        self.assertIn("minHeight: 48", app)
        self.assertIn("AccessibilityInfo.isReduceMotionEnabled", app)
        self.assertIn("PixelRatio.getFontScale", app)

        self.assertIn("APGIC_E2E_ACCESSIBILITY", android_activity)
        self.assertIn("APGIC_E2E_ACCESSIBILITY", ios_delegate)

        self.assertIn("assert_accessibility_and_device_matrix() {", android_script)
        self.assertIn("PHONE_COMPACT", android_script)
        self.assertIn("PHONE_LARGE", android_script)
        self.assertIn("TABLET", android_script)
        self.assertIn("settings put system font_scale 1.30", android_script)
        self.assertIn("android-device-matrix.json", android_script)

        self.assertIn("assert_accessibility_runtime() {", ios_script)
        self.assertIn("--api axbridge", ios_script)
        self.assertIn("content_size accessibility-extra-extra-large", ios_script)
        self.assertIn("ios-accessibility-large-text.json", ios_script)

        for artifact in (
            "evidence/android-accessibility-e2e.xml",
            "evidence/android-device-matrix.json",
            "evidence/ios-accessibility-e2e.json",
            "evidence/ios-accessibility-large-text.json",
        ):
            self.assertIn(artifact, workflow)

    def test_release_remote_config_trust_bootstrap_is_wired(self) -> None:
        gradle = (ROOT / "apps/mobile/android/app/build.gradle").read_text(encoding="utf-8")
        android = (
            ROOT
            / "apps/mobile/android/app/src/main/java/com/apgic/ci/MainActivity.kt"
        ).read_text(encoding="utf-8")
        plist = (ROOT / "apps/mobile/ios/APGIC/Info.plist").read_text(encoding="utf-8")
        ios = (ROOT / "apps/mobile/ios/APGIC/AppDelegate.swift").read_text(encoding="utf-8")

        self.assertIn(
            'buildConfigField "String", "APGIC_REMOTE_CONFIG_TRUSTED_KEY_ID"',
            gradle,
        )
        self.assertIn('it.name == "preReleaseBuild"', gradle)
        self.assertIn("BuildConfig.APGIC_REMOTE_CONFIG_TRUSTED_KEY_ID", android)
        self.assertIn(
            "BuildConfig.APGIC_REMOTE_CONFIG_TRUSTED_PUBLIC_KEY_BASE64",
            android,
        )
        self.assertIn("<key>APGICRemoteConfigTrustedKeyID</key>", plist)
        self.assertIn("<key>APGICRemoteConfigTrustedPublicKeyBase64</key>", plist)
        self.assertIn(
            'forInfoDictionaryKey: "APGICRemoteConfigTrustedKeyID"',
            ios,
        )
        self.assertIn(
            'forInfoDictionaryKey: "APGICRemoteConfigTrustedPublicKeyBase64"',
            ios,
        )


if __name__ == "__main__":
    unittest.main()
