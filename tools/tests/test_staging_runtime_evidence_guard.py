import unittest

from tools.staging_runtime_evidence_guard import (
    discover_evidence_paths,
    validate,
)


class StagingRuntimeEvidenceGuardTest(unittest.TestCase):
    def test_all_staging_runtime_evidence_is_scoped_and_bound(self):
        self.assertEqual(validate(), [])

    def test_guard_discovers_historical_and_current_staging_evidence(self):
        names = {path.name for path in discover_evidence_paths()}
        self.assertIn("staging-runtime-20260926T215500Z.json", names)
        self.assertIn("staging-runtime-20260927T072352Z.json", names)
        self.assertGreaterEqual(len(names), 2)


if __name__ == "__main__":
    unittest.main()
