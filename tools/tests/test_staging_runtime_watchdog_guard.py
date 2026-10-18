import tempfile
import subprocess
import unittest
from pathlib import Path
from unittest.mock import patch

from tools import staging_runtime_watchdog_guard as guard


class StagingRuntimeWatchdogGuardTest(unittest.TestCase):
    def test_finance_staging_shell_scripts_have_valid_bash_syntax(self):
        for name in (
            "check-staging-runtime.sh",
            "update-staging.sh",
            "prove-r4-finance-readonly.sh",
        ):
            script = guard.ROOT / "deploy" / "staging" / name
            with self.subTest(script=name):
                subprocess.run(["bash", "-n", str(script)], check=True, capture_output=True, text=True)

    def test_watchdog_preserves_runtime_invariants(self):
        self.assertEqual(guard.validate(), [])

    def test_missing_finance_health_and_delivery_guards_fail(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            target = root / "deploy" / "staging"
            target.mkdir(parents=True)
            for source in (guard.SCRIPT, guard.SERVICE, guard.TIMER):
                (target / source.name).write_text(source.read_text(encoding="utf-8"), encoding="utf-8")
            original_updater = (guard.ROOT / "deploy/staging/update-staging.sh").read_text(encoding="utf-8")
            (target / "update-staging.sh").write_text(original_updater, encoding="utf-8")
            script = target / guard.SCRIPT.name
            script.write_text(
                script.read_text(encoding="utf-8").replace(
                    "financial schema is incomplete", "financial schema unchecked"
                ).replace(
                    "finance worker requires outbox delivery", "finance worker standalone"
                ),
                encoding="utf-8",
            )
            with patch.object(guard, "ROOT", root), patch.object(guard, "SCRIPT", script), patch.object(guard, "SERVICE", target / guard.SERVICE.name), patch.object(guard, "TIMER", target / guard.TIMER.name):
                errors = guard.validate()
                self.assertTrue(any("finance watchdog missing invariant" in e for e in errors), errors)

    def test_missing_predeployment_dependency_rejected(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            target = root / "deploy" / "staging"
            target.mkdir(parents=True)
            for source in (guard.SCRIPT, guard.SERVICE, guard.TIMER):
                (target / source.name).write_text(source.read_text(encoding="utf-8"), encoding="utf-8")
            updater = target / "update-staging.sh"
            updater.write_text(
                (guard.ROOT / "deploy/staging/update-staging.sh").read_text(encoding="utf-8").replace(
                    "APGIC finance worker activation requires APGIC_OUTBOX_WORKER_ENABLED=1",
                    "finance worker can activate without payout delivery",
                ),
                encoding="utf-8",
            )
            with patch.object(guard, "ROOT", root), patch.object(guard, "SCRIPT", target / guard.SCRIPT.name), patch.object(guard, "SERVICE", target / guard.SERVICE.name), patch.object(guard, "TIMER", target / guard.TIMER.name):
                errors = guard.validate()
                self.assertTrue(any("staging deploy missing finance dependency" in e for e in errors), errors)


if __name__ == "__main__":
    unittest.main()
