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

    def test_android_sdk_install_retries_transient_corrupt_downloads(self) -> None:
        android = self._read("mobile_android_capability_e2e.sh")

        self.assertIn("install_android_sdk_packages() {", android)
        self.assertIn("for attempt in 1 2 3; do", android)
        self.assertIn('rm -rf "$HOME/.android/cache"', android)
        self.assertIn(
            'fail "Android SDK package installation failed after bounded retries"',
            android,
        )

    def test_android_e2e_budget_and_new_evidence_are_bounded(self) -> None:
        app = (ROOT / "apps/mobile/src/App.tsx").read_text(encoding="utf-8")
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
            "/sdcard/apgic-deletion-e2e.xml",
        ):
            self.assertEqual(android.count("dump_until_labels_visible " + path), 1)
        self.assertIn('local remote="/sdcard/apgic-demand-e2e.xml"', android)
        self.assertIn('APGIC_E2E_DEMAND_SESSION_COOKIE "$SESSION_COOKIE"', android)
        # Mobile v0.87 renders a long, scrollable user journey. Assert the
        # native automation checks actual on-screen accessibility nodes and
        # both states of each checkbox, rather than relying on an obsolete
        # direct-tap helper that could tap an off-screen coordinate.
        self.assertIn("seek_demand_accessibility_node() {", android)
        self.assertIn('"$ADB" shell uiautomator dump "$remote"', android)
        # The parser exercises actual on-screen bounds, state and visible
        # human-facing evidence. It must reject clipped/outside-view nodes
        # while accepting the real lower-edge Android diagnosis node.
        locator = self._read("android_ui_accessibility.py")
        self.assertIn('matches_semantics(attributes, label)', locator)
        self.assertIn('attributes.get("checked") == checked', locator)
        self.assertIn('action == "observe" or attributes.get("enabled") == "true"', locator)
        self.assertIn('attributes.get("clickable") == "true"', locator)
        self.assertIn('minimum_height = 28 if action == "tap" else 12', locator)
        self.assertIn('intersection(rectangle, viewport)', locator)
        self.assertIn('android_ui_accessibility.py" "$output" "$label" "$checked" "$action"', android)
        self.assertIn('seek_demand_accessibility_node "С чем нужна помощь" "" tap', android)
        self.assertIn("seek_demand_accessibility_node 'native-demand-diagnosis:false' \"\" observe", android)
        self.assertIn("seek_demand_accessibility_node 'Тема anxiety' true tap", android)
        self.assertIn("seek_demand_accessibility_node 'Тема anxiety' false observe", android)
        self.assertIn("seek_demand_accessibility_node 'Тема sleep' true observe", android)
        self.assertIn('seek_demand_accessibility_node "Подтвердить темы запроса" "" tap', android)
        self.assertIn("seek_demand_accessibility_node 'native-demand-status:FAIL'", android)
        self.assertIn("press_accepted", android)
        # The script must report failure when no real UI status transition
        # occurs, including the observed native onPress count.
        self.assertIn('if [[ "$press_accepted" != true ]]; then', android)
        self.assertIn('fail "Android confirmation did not leave DRAFT', android)
        self.assertIn('APGIC_CONFIRM_PRESS', android)
        self.assertIn('dismiss_android_ime || fail', android)
        self.assertIn('APGIC_CONFIRM_PRESS_IN', android)
        self.assertIn('APGIC_CONFIRM_PRESS_OUT', android)
        self.assertIn('android-confirm-failure.png', android)
        self.assertIn('android-confirm-failure.xml', android)
        self.assertIn('android-confirm-before-tap.xml', android)
        self.assertIn('android-confirm-before-tap.png', android)
        self.assertIn('shell input touchscreen tap $position', android)
        self.assertIn('setDemandTouchEvidence(v => ({...v, down: v.down + 1}))', app)
        self.assertIn('native-demand-touch:down:', app)
        self.assertIn('setDemandTouchEvidence(v => ({...v, up: v.up + 1}))', app)
        self.assertIn('setDemandTouchEvidence(v => ({...v, press: v.press + 1}))', app)
        self.assertIn('Android UI-visible touch evidence:', android)

        self.assertIn('[[ "$confirm_state" == "CONFIRMED" ]]', android)
        self.assertIn("seek_demand_accessibility_node 'native-demand-confirmed' \"\" observe", android)
        self.assertIn("seek_demand_accessibility_node 'Тема anxiety' false observe", android)
        self.assertIn('fail "Android confirmed HelpIntent silently reselected', android)

    def test_demand_e2e_matches_user_help_intent_execution_path(self) -> None:
        app = (ROOT / "apps/mobile/src/App.tsx").read_text(encoding="utf-8")

        demand_effect_marker = app.index("setDemandE2E({status: \"RUNNING\"})")
        demand_effect_start = app.rindex("  useEffect(() => {", 0, demand_effect_marker)
        demand_effect_end = app.index("  ]);", demand_effect_marker)
        demand_effect = app[demand_effect_start:demand_effect_end]
        self.assertIn("!compatibilityAllowsRuntime", demand_effect)
        self.assertIn("compatibilityAllowsRuntime,", demand_effect)
        self.assertIn("const demandE2EActive = Boolean(", app)
        self.assertIn("demandE2EActive ||", app)
        self.assertIn("(compatibilityBaseURL !== canonicalAPGICOrigin && !demandE2EBaseURL) ||", app)

    def test_production_demand_mutations_fail_closed_and_allow_topic_replacement(self) -> None:
        app = (ROOT / "apps/mobile/src/App.tsx").read_text(encoding="utf-8")
        android = self._read("mobile_android_capability_e2e.sh")

        analyze_start = app.index("  const analyzeDemand = useCallback")
        analyze_end = app.index("  const toggleDemandTopic", analyze_start)
        analyze = app[analyze_start:analyze_end]
        self.assertIn("if (!compatibilityAllowsRuntime)", analyze)

        confirm_start = app.index("  const confirmDemand = useCallback")
        confirm_end = app.index("  const handleDeepLink", confirm_start)
        confirm = app[confirm_start:confirm_end]
        self.assertIn("if (!compatibilityAllowsRuntime)", confirm)

        self.assertIn('accessibilityLabel="Добавить свою тему запроса"', app)
        self.assertIn("const addDemandTopic = useCallback", app)
        self.assertIn("current.includes(topic) ? current : [...current, topic]", app)
        self.assertIn("...demandTopics", app)
        self.assertIn('native-demand-status:${demandStatus}', app)
        self.assertIn('accessibilityLabel="native-demand-error"', app)
        self.assertIn("confirmed.id !== demandIntent.id", app)
        self.assertIn("confirmed.client_identity_id !== demandIntent.client_identity_id", app)
        self.assertIn("!demandIntent ? (", app)
        self.assertIn("setDemandSuggestedTopics([...intent.topics])", app)
        self.assertIn("...demandSuggestedTopics", app)
        self.assertIn('disabled={demandStatus === "CONFIRMED" || demandStatus === "RUNNING"}', app)
        self.assertIn("setDemandCustomTopicEditing((value) => !value)", app)
        self.assertIn('disabled={demandStatus === "CONFIRMED" || demandStatus === "RUNNING"}', app)
        self.assertIn('disabled={!demandCustomTopic.trim() || demandStatus === "RUNNING"}', app)
        self.assertIn("shell input swipe 300 540 300 180 600", android)
        self.assertIn("shell input swipe 300 180 300 540 600", android)
        self.assertNotIn("shell input swipe 160 120 160 555 95", android)
        # The locator must observe settled native coordinates rather than
        # relying on a hardcoded legacy sleep interval.
        self.assertIn('local previous_position="$position" stable=false settle', android)
        self.assertIn('[[ "$position" == "$previous_position" ]]', android)
        self.assertIn('[[ "$stable" == true && -n "$position" ]] || continue', android)
        self.assertIn('confirm_count_after > confirm_count_before', android)
        self.assertIn('session_cookie_present=true', android)
        self.assertIn('accessibilityLabel="Создать новый запрос"', app)
        self.assertIn('keyboardShouldPersistTaps="always"', app)
        self.assertIn('keyboardDismissMode="on-drag"', app)
        self.assertIn('style={styles.demandAction}', app)
        self.assertNotIn('style={styles.demandStickyFooter}', app)
        self.assertNotIn('demandScrollAboveFooter', app)
        self.assertIn('APGIC_CONFIRM_PRESS', app)
        self.assertIn('logcat -c', android)
        self.assertIn('APGIC_CONFIRM_PRESS', android)
        self.assertIn('previous_position', android)
        self.assertNotIn("flexGrow: 0", app)
        self.assertNotIn("onPressIn={() => void confirmDemand()}", app)
        self.assertNotIn('keyboardShouldPersistTaps="handled"', app)
        self.assertEqual(app.count('accessibilityLabel="Подтвердить темы запроса"'), 1)
        # One real, in-flow confirmation action, before ScrollView closes.
        label = app.index('accessibilityLabel="Подтвердить темы запроса"')
        self.assertLess(label, app.index("      </ScrollView>"))
        control = app[app.rfind("<Pressable", 0, label):app.index("</Pressable>", label)]
        self.assertIn('void confirmDemand()', control)
        self.assertIn('disabled={demandStatus === "RUNNING" || demandTopics.length === 0 || !compatibilityAllowsRuntime}', control)

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


    def test_ios_installed_e2e_uses_embedded_bundle_not_metro(self) -> None:
        script = self._read("mobile_ios_capability_e2e.sh")
        delegate = (
            ROOT / "apps/mobile/ios/APGIC/AppDelegate.swift"
        ).read_text(encoding="utf-8")

        self.assertIn('EMBEDDED_BUNDLE="$APP/main.jsbundle"', script)
        self.assertIn('[[ -s "$EMBEDDED_BUNDLE" ]]', script)
        self.assertIn("export SIMCTL_CHILD_APGIC_E2E_USE_EMBEDDED_BUNDLE=true", script)
        self.assertNotIn("127.0.0.1:8081", script)
        self.assertNotIn("APGIC_E2E_METRO_URL", script)

        self.assertIn(
            'environment["APGIC_E2E_USE_EMBEDDED_BUNDLE"] == "true"',
            delegate,
        )
        self.assertIn(
            'Bundle.main.url(forResource: "main", withExtension: "jsbundle")',
            delegate,
        )
        self.assertIn('environment["APGIC_E2E_METRO_URL"]', delegate)
        self.assertIn('RCTBundleURLProvider.sharedSettings().jsBundleURL(forBundleRoot: "index")', delegate)

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
        self.assertIn(
            'json_has_ax_label_fragment "$target" "realtime-audio-route-observed:" "BLUETOOTH"',
            realtime,
        )
        self.assertNotIn(
            'json_has_ax_label "$target" "realtime-audio-route:BLUETOOTH"',
            realtime,
        )
        self.assertIn(
            'json_has_ax_label_fragment "$target" "realtime-network-transport-observed:" "CELLULAR"',
            realtime,
        )
        self.assertNotIn(
            'json_has_ax_label "$target" "realtime-network-transport:CELLULAR"',
            realtime,
        )
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
