from __future__ import annotations

import subprocess
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
HARNESS = ROOT / "tools/mobile_ios_capability_e2e.sh"


class IOSCapabilityRegistrationRecoveryTest(unittest.TestCase):
    """Exercise actual shell helper against hermetic simctl failures (no macOS required)."""

    def run_recovery(self, mode: str) -> subprocess.CompletedProcess[str]:
        source = HARNESS.read_text(encoding="utf-8")
        begin = source.index("launch_installed_capability_state() {")
        end = source.index("\n}\n", begin) + 2
        helper = source[begin:end]
        script = r"""
set -euo pipefail
UDID=test-simulator-udid
APP=/fake/APGIC.app
INSTALLS=0
LAUNCHES=0
GETS=0
sleep() { :; }
fail() { echo "HARD_FAIL $*" >&2; exit 92; }
xcrun() {
  if [[ "$1" != "simctl" ]]; then return 78; fi
  shift
  case "$1" in
    get_app_container)
      GETS=$((GETS + 1))
      if [[ "$MOCK_MODE" == unregistered && "$GETS" == 1 ]]; then
        return 1
      fi
      return 0
      ;;
    install)
      INSTALLS=$((INSTALLS + 1))
      return 0
      ;;
    launch)
      LAUNCHES=$((LAUNCHES + 1))
      if [[ "$MOCK_MODE" == notfound-once && "$LAUNCHES" == 1 ||
            "$MOCK_MODE" == notfound-always ]]; then
        echo 'NotFound ("Application "com.apgic.ci" is unknown to FrontBoard")' >&2
        return 1
      fi
      if [[ "$MOCK_MODE" == application-crash ]]; then
        echo 'Failed to launch: app terminated due to native crash' >&2
        return 1
      fi
      [[ "$SIMCTL_CHILD_APGIC_E2E_CAPABILITY_STATE" == RESTRICTED ]] || return 93
      return 0
      ;;
    *) return 78 ;;
  esac
}
"""
        script += "\n" + helper + "\n"
        script += """set +e
launch_installed_capability_state RESTRICTED
status=$?
echo "RECOVERY_RESULT=$status INSTALLS=$INSTALLS LAUNCHES=$LAUNCHES GETS=$GETS"
exit "$status"
"""
        return subprocess.run(
            ["bash", "-c", script],
            capture_output=True,
            text=True,
            env={"MOCK_MODE": mode, "PATH": "/usr/bin:/bin"},
            check=False,
        )

    def test_launch_succeeds_once_registration_exists(self) -> None:
        result = self.run_recovery("healthy")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("INSTALLS=0 LAUNCHES=1", result.stdout)

    def test_not_registered_install_is_restored_and_state_preserved(self) -> None:
        result = self.run_recovery("unregistered")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("INSTALLS=1 LAUNCHES=1", result.stdout)

    def test_frontboard_notfound_is_retried_with_real_reinstall(self) -> None:
        result = self.run_recovery("notfound-once")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("INSTALLS=1 LAUNCHES=2", result.stdout)

    def test_repeated_notfound_fails_closed_after_bounded_retries(self) -> None:
        result = self.run_recovery("notfound-always")
        self.assertEqual(result.returncode, 1)
        self.assertIn("INSTALLS=2 LAUNCHES=3", result.stdout)

    def test_real_crash_is_not_treated_as_installation_race(self) -> None:
        result = self.run_recovery("application-crash")
        self.assertEqual(result.returncode, 1)
        self.assertIn("INSTALLS=0 LAUNCHES=1", result.stdout)
