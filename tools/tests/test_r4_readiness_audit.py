import unittest
from r4_readiness_audit import REQUIRED, CRITICAL_GATES, audit

SHA = "a" * 40

class ReadinessTests(unittest.TestCase):
    def valid_inputs(self):
        registry = {"requirements": [
            {"requirement_id": key, "status": "VERIFIED", "evidence_refs": ["evidence://foo"]}
            for key in REQUIRED
        ]}
        manifest = {
            "schema_version": "r4-release-gate-v1", "candidate_sha": SHA,
            "synthetic": False, "production_candidate": True,
            "evidence": {key: {"status": "PASS", "ref": "evidence://fake"}
                         for key in CRITICAL_GATES},
        }
        return registry, manifest

    def test_empty_manifest_fails_closed(self):
        result = audit({"requirements": []}, {}, SHA)
        self.assertFalse(result["ready"])
        self.assertIn("SYNTHETIC_OR_UNDECLARED_EVIDENCE", result["blockers"])

    def test_unverified_registry_never_passes(self):
        registry, manifest = self.valid_inputs()
        registry["requirements"][0]["status"] = "IN_PROGRESS"
        self.assertFalse(audit(registry, manifest, SHA)["inventory_complete"])

    def test_forged_evidence_references_never_authorize_release(self):
        registry, manifest = self.valid_inputs()
        report = audit(registry, manifest, SHA)
        self.assertTrue(report["inventory_complete"])
        self.assertFalse(report["ready"])
        self.assertIn("AUTHORITATIVE_PRODUCTION_RELEASE_GATE_REQUIRED", report["blockers"])

    def test_flat_string_evidence_shape_is_rejected(self):
        registry, manifest = self.valid_inputs()
        manifest["evidence"] = {key: "evidence://fake" for key in CRITICAL_GATES}
        self.assertFalse(audit(registry, manifest, SHA)["inventory_complete"])

    def test_duplicate_requirement_fails_closed(self):
        registry, manifest = self.valid_inputs()
        registry["requirements"].append(dict(registry["requirements"][0]))
        self.assertIn(f"{REQUIRED[0]}:DUPLICATE_REQUIREMENT",
                      audit(registry, manifest, SHA)["blockers"])

    def test_invalid_root_type_fails_closed(self):
        self.assertIn("REGISTRY_INVALID", audit([], {}, SHA)["blockers"])
        self.assertIn("MANIFEST_INVALID", audit({}, [], SHA)["blockers"])

    def test_invalid_candidate_sha_fails_closed(self):
        registry, manifest = self.valid_inputs()
        self.assertIn("EXPECTED_COMMIT_SHA_INVALID",
                      audit(registry, manifest, "not-sha")["blockers"])

if __name__ == "__main__":
    unittest.main()
