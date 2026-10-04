import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]


class WebCiPortGuardTest(unittest.TestCase):
    def test_web_ci_port_stays_outside_linux_ephemeral_range(self):
        config = (ROOT / "apps/web/playwright.config.ts").read_text(encoding="utf-8")
        self.assertIn("const ciWebPortBase = 18000;", config)
        self.assertIn("ciWebPortBase + ciOffset", config)
        self.assertNotIn("43000 + ciOffset", config)

    def test_api_ephemeral_picker_excludes_web_port(self):
        script = (ROOT / "apps/web/scripts/start-with-api.sh").read_text(
            encoding="utf-8"
        )
        self.assertIn('APGIC_WEB_PORT="$APGIC_WEB_PORT" node', script)
        self.assertIn('const forbiddenPort = Number(process.env.APGIC_WEB_PORT || "0");', script)
        self.assertIn("if (port === forbiddenPort)", script)


if __name__ == "__main__":
    unittest.main()
