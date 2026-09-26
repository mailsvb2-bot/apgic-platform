import copy
import json
import unittest

from tools.restore_evidence_contract_guard import (
    SCHEMA,
    SCRIPT,
    WORKFLOW,
    validate_restore_evidence_contract,
)


class RestoreEvidenceContractGuardTest(unittest.TestCase):
    def setUp(self):
        self.script = SCRIPT.read_text(encoding="utf-8")
        self.workflow = WORKFLOW.read_text(encoding="utf-8")
        self.schema = json.loads(SCHEMA.read_text(encoding="utf-8"))

    def test_repository_evidence_matches_schema_and_ci(self):
        self.assertEqual(validate_restore_evidence_contract(self.script, self.workflow, self.schema), [])

    def test_missing_emitted_field_is_rejected(self):
        broken = copy.deepcopy(self.schema)
        broken["required"].remove("candidate_sha")
        errors = validate_restore_evidence_contract(self.script, self.workflow, broken)
        self.assertTrue(any("JSON keys differ" in error for error in errors))

    def test_ci_evidence_cannot_claim_production(self):
        broken = copy.deepcopy(self.schema)
        broken["properties"]["production_evidence"]["const"] = True
        errors = validate_restore_evidence_contract(self.script, self.workflow, broken)
        self.assertTrue(any("must not claim production" in error for error in errors))

    def test_uploaded_restore_artifact_is_required(self):
        broken_workflow = self.workflow.replace("path: evidence/restore-drill.json", "path: evidence/missing.json")
        errors = validate_restore_evidence_contract(self.script, broken_workflow, self.schema)
        self.assertTrue(any("CI invariant missing" in error for error in errors))


if __name__ == "__main__":
    unittest.main()
