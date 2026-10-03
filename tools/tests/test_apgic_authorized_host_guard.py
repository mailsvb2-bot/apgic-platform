import os
import subprocess
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
GUARD = ROOT / "deploy/staging/assert-authorized-host.sh"


class APGICAuthorizedHostGuardTest(unittest.TestCase):
    def _run_with_ip(self, ipv4: str) -> subprocess.CompletedProcess[str]:
        with tempfile.TemporaryDirectory() as tmp:
            fake_ip = Path(tmp) / "ip"
            fake_ip.write_text(
                "#!/usr/bin/env bash\n"
                "if [[ \"$*\" == \"-4 -o addr show scope global\" ]]; then\n"
                f"  echo \"2: eth0    inet {ipv4}/24 brd 255.255.255.255 scope global eth0\"\n"
                "  exit 0\n"
                "fi\n"
                "exit 1\n",
                encoding="utf-8",
            )
            fake_ip.chmod(0o755)
            env = os.environ.copy()
            env["PATH"] = f"{tmp}:{env['PATH']}"
            return subprocess.run(
                ["bash", str(GUARD)],
                text=True,
                capture_output=True,
                env=env,
                check=False,
            )

    def test_authorized_apgic_host_passes(self):
        result = self._run_with_ip("92.51.23.254")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("PASS ipv4=92.51.23.254", result.stdout)

    def test_any_other_host_fails_closed(self):
        result = self._run_with_ip("217.198.12.191")
        self.assertEqual(result.returncode, 78)
        self.assertIn("host boundary violation", result.stderr)
        self.assertIn("92.51.23.254", result.stderr)


if __name__ == "__main__":
    unittest.main()
