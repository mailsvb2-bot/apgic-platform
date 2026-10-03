import unittest
from pathlib import Path
from tools.staging_boundary_guard import validate

ROOT = Path(__file__).resolve().parents[2]

class StagingBoundaryGuardTest(unittest.TestCase):
    def test_repository_staging_boundary_is_isolated(self):
        self.assertEqual(validate(), [])

    def test_host_guard_stays_on_server_entrypoints_not_ci_migration_helper(self):
        guarded = (
            "update-staging.sh",
            "check-staging-runtime.sh",
            "bootstrap-dedicated-host.sh",
            "backup-staging-postgres.sh",
            "verify-staging-backup.sh",
        )
        for name in guarded:
            text = (ROOT / "deploy/staging" / name).read_text(encoding="utf-8")
            self.assertIn("assert-authorized-host.sh", text, name)

        helper = (ROOT / "deploy/staging/apply-staging-migrations.sh").read_text(
            encoding="utf-8"
        )
        self.assertNotIn("assert-authorized-host.sh", helper)

if __name__ == "__main__":
    unittest.main()
