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


if __name__ == "__main__":
    unittest.main()
