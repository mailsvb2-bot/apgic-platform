import copy
import unittest
import yaml

from tools.test_canon_contract_guard import (
    CONTRACT,
    WORKFLOW,
    validate_test_canon_contract,
)


class TestCanonContractGuardTest(unittest.TestCase):
    def setUp(self):
        self.doc = yaml.safe_load(CONTRACT.read_text(encoding="utf-8")) or {}
        self.workflow = WORKFLOW.read_text(encoding="utf-8")

    def test_repository_test_canon_matches_executable_evidence(self):
        self.assertEqual(validate_test_canon_contract(self.doc, self.workflow), [])

    def test_missing_required_class_is_rejected(self):
        broken = copy.deepcopy(self.doc)
        broken["required_classes"].pop("RECOVERY")
        errors = validate_test_canon_contract(broken, self.workflow)
        self.assertTrue(any("classes differ" in error for error in errors))

    def test_missing_evidence_file_is_rejected(self):
        broken = copy.deepcopy(self.doc)
        broken["required_classes"]["CONCURRENCY"]["evidence"] = ["backend/ci/missing.sh"]
        errors = validate_test_canon_contract(broken, self.workflow)
        self.assertTrue(any("missing evidence file" in error for error in errors))

    def test_missing_ci_invocation_is_rejected(self):
        broken_workflow = self.workflow.replace("go test -race ./...", "go test ./...")
        errors = validate_test_canon_contract(self.doc, broken_workflow)
        self.assertTrue(any("required CI invocation missing" in error for error in errors))


if __name__ == "__main__":
    unittest.main()
