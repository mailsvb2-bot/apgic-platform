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
        self.assertIn("boot_simulator_with_recovery() {", text)
        self.assertIn("for boot_attempt in 1 2; do", text)
        self.assertIn('grep -q "Data Migration Failed" "$SIMULATOR_BOOT_LOG"', text)
        self.assertIn('xcrun simctl erase "$UDID"', text)
        self.assertIn(
            "failed clean boot after bounded migration recovery",
            text,
        )
        self.assertIn('cat "$supported_output" >&2', text)

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
