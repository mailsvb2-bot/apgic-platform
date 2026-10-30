"""Keep the staging release gate fail-closed on R4 and real R0-R3 proof."""
from pathlib import Path
import subprocess
import unittest

ROOT = Path(__file__).resolve().parents[2]
GATE = ROOT / "deploy/staging/prove-launch-r0-r4.sh"


class LaunchGateContractTests(unittest.TestCase):
    def test_shell_syntax(self):
        subprocess.run(["bash", "-n", str(GATE)], check=True)

    def test_launch_requires_live_finance_and_outbox(self):
        script = GATE.read_text(encoding="utf-8")
        self.assertIn("APGIC_FINANCE_WORKER_ENABLED", script)
        self.assertIn("APGIC_OUTBOX_WORKER_ENABLED", script)
        self.assertIn("LAUNCH BLOCKED", script)
        self.assertIn("prove-r0-r3-dual-party.sh", script)
        self.assertIn("--gate all --mode production", script)
        self.assertIn("check-staging-runtime.sh", script)
        self.assertNotIn("--mode ci", script)


if __name__ == "__main__":
    unittest.main()
